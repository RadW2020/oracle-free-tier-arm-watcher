package api

import (
	"context"
	"sync"
	"time"

	"github.com/RadW2020/oracle-free-tier-arm-watcher/internal/audit"
	"github.com/RadW2020/oracle-free-tier-arm-watcher/internal/snapshot"
	"github.com/RadW2020/oracle-free-tier-arm-watcher/internal/source"
	"golang.org/x/time/rate"
)

// APIVersion es la versión del contrato /v1 y de las tools MCP.
const APIVersion = "v1"

// Límites de la línea de tiempo. Protegen a OCI (cada consulta son hasta
// cinco llamadas a Monitoring) y al contexto del agente: 720 puntos en JSON
// son ~30 KB, lo máximo razonable que una respuesta debería meterle a un
// modelo de una vez.
const (
	RetentionDays             = 90
	MaxPointsPerResponse      = 720
	TimelineRequestsPerMinute = 20
)

// MaxWindow es la ventana máxima por resolución.
var MaxWindow = map[Resolution]time.Duration{
	Resolution1m: 24 * time.Hour,
	Resolution5m: 7 * 24 * time.Hour,
	Resolution1h: RetentionDays * 24 * time.Hour,
}

// Config configura el servicio.
type Config struct {
	Store           *snapshot.Store
	Audit           *audit.Log
	Registry        *audit.Registry
	Version         string
	RefreshInterval time.Duration
}

// Service implementa las operaciones.
type Service struct {
	cfg Config
	src source.Source

	limMu    sync.Mutex
	limiters map[string]*rate.Limiter
}

// New crea el servicio.
func New(cfg Config) *Service {
	if cfg.RefreshInterval == 0 {
		cfg.RefreshInterval = 15 * time.Minute
	}
	return &Service{cfg: cfg, src: cfg.Store.Source(), limiters: map[string]*rate.Limiter{}}
}

// Registry devuelve el registro de clientes, para los transportes.
func (s *Service) Registry() *audit.Registry { return s.cfg.Registry }

// Audit devuelve el registro de auditoría.
func (s *Service) Audit() *audit.Log { return s.cfg.Audit }

// call envuelve una operación: comprueba el permiso, la ejecuta y deja el
// evento de auditoría, con éxito o sin él. Es el único sitio por el que pasa
// cualquier operación, venga de REST o de MCP.
func (s *Service) call(ctx context.Context, op string, scope audit.Scope, args map[string]any, fn func() ([]string, error)) error {
	start := time.Now()
	client, ok := audit.ClientFrom(ctx)
	var (
		err      error
		affected []string
	)
	switch {
	case !ok:
		err = &Error{Code: CodeUnauthenticated, Message: "no authenticated client", Hint: "send Authorization: Bearer <key> or X-API-Key: <key>"}
	case !client.Has(scope):
		err = denied(scope)
	default:
		affected, err = fn()
	}

	event := audit.Event{
		Time:       start.UTC(),
		RequestID:  audit.RequestID(ctx),
		Client:     client.Name,
		Transport:  audit.Transport(ctx),
		Session:    audit.Session(ctx),
		Operation:  op,
		Args:       args,
		Outcome:    audit.OutcomeOK,
		DurationMs: time.Since(start).Milliseconds(),
		DataSource: s.src.Kind(),
		Affected:   affected,
	}
	if err != nil {
		apiErr := AsError(err)
		event.ErrorCode = string(apiErr.Code)
		event.Outcome = audit.OutcomeError
		if apiErr.Code == CodePermissionDenied || apiErr.Code == CodeUnauthenticated || apiErr.Code == CodeRateLimited {
			event.Outcome = audit.OutcomeDenied
		}
		err = apiErr
	}
	if s.cfg.Audit != nil {
		s.cfg.Audit.Record(event)
	}
	return err
}

// RecordRejected deja constancia de una petición rechazada antes de llegar a
// una operación (sin clave, clave inválida, método incorrecto).
func (s *Service) RecordRejected(ctx context.Context, op string, code ErrorCode) {
	if s.cfg.Audit == nil {
		return
	}
	client := "unauthenticated"
	if c, ok := audit.ClientFrom(ctx); ok {
		client = c.Name
	}
	s.cfg.Audit.Record(audit.Event{
		Time: time.Now().UTC(), RequestID: audit.RequestID(ctx), Client: client, Transport: audit.Transport(ctx),
		Operation: op, Outcome: audit.OutcomeDenied, ErrorCode: string(code), DataSource: s.src.Kind(),
	})
}

// meta compone el bloque meta de una respuesta.
func (s *Service) meta(ctx context.Context, snap *snapshot.Snapshot, complete bool) Meta {
	m := Meta{
		RequestID:   audit.RequestID(ctx),
		APIVersion:  APIVersion,
		DataSource:  s.src.Kind(),
		Scope:       s.src.Scope(),
		GeneratedAt: s.src.Now(),
		Complete:    complete,
	}
	if f, ok := s.src.(*source.Fixture); ok {
		m.Scenario = f.Scenario().Name
	}
	if snap != nil {
		observed := snap.Reading.ObservedAt
		age := int(s.src.Now().Sub(observed).Seconds())
		m.ObservedAt = &observed
		m.SnapshotAgeSeconds = &age
	}
	return m
}

// currentSnapshot devuelve la lectura guardada, o el error que explica por
// qué no la hay.
func (s *Service) currentSnapshot() (snapshot.Snapshot, error) {
	if snap, ok := s.cfg.Store.Current(); ok {
		return snap, nil
	}
	if !s.src.Configured() {
		return snapshot.Snapshot{}, fromSource(source.ErrNotConfigured)
	}
	h := s.cfg.Store.Health()
	if h.LastError != nil {
		e := fromSource(h.LastError)
		e.Message = "no snapshot has been read successfully yet: " + e.Message
		return snapshot.Snapshot{}, e
	}
	return snapshot.Snapshot{}, &Error{
		Code: CodeSourceUnavailable, Retryable: true, RetryAfterSeconds: 10,
		Message: "the first read from OCI is still in progress",
	}
}

// stale dice si la lectura guardada es más vieja de lo que debería.
func (s *Service) stale(snap snapshot.Snapshot) bool {
	return s.src.Now().Sub(snap.Reading.ObservedAt) > 2*s.cfg.RefreshInterval
}

// limiter devuelve el limitador de un cliente para la línea de tiempo.
func (s *Service) limiter(client string) *rate.Limiter {
	s.limMu.Lock()
	defer s.limMu.Unlock()
	l, ok := s.limiters[client]
	if !ok {
		l = rate.NewLimiter(rate.Every(time.Minute/TimelineRequestsPerMinute), 10)
		s.limiters[client] = l
	}
	return l
}
