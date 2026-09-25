// Package snapshot guarda la última lectura de la tenancy y controla cuándo
// se vuelve a pedir.
//
// El worker de métricas ya leía OCI cada 15 minutos, pero tiraba el
// resultado salvo para los gauges de Prometheus: cada petición a /usage
// volvía a hacer 13 + N llamadas a OCI. Un agente que preguntase en bucle
// multiplicaba esa carga contra la API que vigila la factura. Ahora la
// lectura del worker se guarda y la API /v1 la sirve desde aquí; una lectura
// nueva sólo se pide con refresh, con cooldown y agrupando peticiones
// simultáneas en una sola.
package snapshot

import (
	"context"
	"sync"
	"time"

	"github.com/RadW2020/oracle-free-tier-arm-watcher/internal/freetier"
	"github.com/RadW2020/oracle-free-tier-arm-watcher/internal/source"
)

// Snapshot es una lectura guardada.
type Snapshot struct {
	Reading   freetier.Reading
	FetchedAt time.Time
	Duration  time.Duration
}

// Motivos de un Refresh.
const (
	ReasonRefreshed = "refreshed"
	ReasonCoalesced = "coalesced"
	ReasonCooldown  = "cooldown"
)

// RefreshResult dice qué pasó al pedir una lectura nueva.
type RefreshResult struct {
	Refreshed  bool
	Reason     string
	RetryAfter time.Duration
	Snapshot   *Snapshot
	Err        error
}

// Health es el estado del store, para el diagnóstico del watcher.
type Health struct {
	Snapshot      *Snapshot
	LastAttemptAt time.Time
	LastSuccessAt time.Time
	LastError     error
	Refreshing    bool
}

// Options configura el store.
type Options struct {
	// Cooldown es el tiempo mínimo entre dos lecturas pedidas desde fuera.
	Cooldown time.Duration
	// Timeout acota una lectura completa.
	Timeout time.Duration
	// OnUpdate se llama tras cada lectura buena (los gauges de Prometheus).
	OnUpdate func(freetier.Reading)
	// OnError se llama tras cada lectura fallida (el log).
	OnError func(error)
	// Now es el reloj de pared, sustituible en tests.
	Now func() time.Time
}

// Store guarda la última lectura.
type Store struct {
	src  source.Source
	opts Options

	mu          sync.Mutex
	current     *Snapshot
	lastAttempt time.Time
	lastSuccess time.Time
	lastErr     error
	inflight    chan struct{}
}

// New crea un store vacío.
func New(src source.Source, opts Options) *Store {
	if opts.Cooldown == 0 {
		opts.Cooldown = time.Minute
	}
	if opts.Timeout == 0 {
		opts.Timeout = 60 * time.Second
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	return &Store{src: src, opts: opts}
}

// Source devuelve la fuente de la que lee.
func (s *Store) Source() source.Source { return s.src }

// Cooldown devuelve el tiempo mínimo entre lecturas pedidas desde fuera.
func (s *Store) Cooldown() time.Duration { return s.opts.Cooldown }

// Current devuelve la última lectura buena, si la hay.
func (s *Store) Current() (Snapshot, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.current == nil {
		return Snapshot{}, false
	}
	return *s.current, true
}

// Health devuelve el estado del store.
func (s *Store) Health() Health {
	s.mu.Lock()
	defer s.mu.Unlock()
	h := Health{
		LastAttemptAt: s.lastAttempt,
		LastSuccessAt: s.lastSuccess,
		LastError:     s.lastErr,
		Refreshing:    s.inflight != nil,
	}
	if s.current != nil {
		snap := *s.current
		h.Snapshot = &snap
	}
	return h
}

// Refresh pide una lectura nueva.
//
// Si ya hay una en curso, espera a esa en vez de lanzar otra (coalesced).
// Si la última se pidió hace menos del cooldown y force es false, devuelve
// la que hay y cuánto falta (cooldown): para quien llama, un dato de hace
// un minuto es la respuesta útil, no un error.
func (s *Store) Refresh(ctx context.Context, force bool) RefreshResult {
	s.mu.Lock()
	if ch := s.inflight; ch != nil {
		s.mu.Unlock()
		select {
		case <-ch:
		case <-ctx.Done():
			return RefreshResult{Reason: ReasonCoalesced, Err: ctx.Err()}
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		return RefreshResult{Refreshed: s.lastErr == nil, Reason: ReasonCoalesced, Snapshot: s.copyCurrent(), Err: s.lastErr}
	}
	now := s.opts.Now()
	if !force && !s.lastAttempt.IsZero() {
		if wait := s.opts.Cooldown - now.Sub(s.lastAttempt); wait > 0 {
			defer s.mu.Unlock()
			return RefreshResult{Reason: ReasonCooldown, RetryAfter: wait, Snapshot: s.copyCurrent()}
		}
	}
	ch := make(chan struct{})
	s.inflight = ch
	s.lastAttempt = now
	s.mu.Unlock()

	// La lectura no hereda la cancelación de quien la pidió: otros pueden
	// estar esperándola, y a medias no le sirve a nadie.
	readCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.opts.Timeout)
	start := s.opts.Now()
	reading, err := s.src.Read(readCtx)
	cancel()
	duration := s.opts.Now().Sub(start)

	s.mu.Lock()
	s.inflight = nil
	close(ch)
	if err != nil {
		s.lastErr = err
	} else {
		s.lastErr = nil
		s.lastSuccess = s.opts.Now()
		s.current = &Snapshot{Reading: reading, FetchedAt: s.lastSuccess, Duration: duration}
	}
	result := RefreshResult{Refreshed: err == nil, Reason: ReasonRefreshed, Snapshot: s.copyCurrent(), Err: err}
	s.mu.Unlock()

	if err != nil {
		if s.opts.OnError != nil {
			s.opts.OnError(err)
		}
	} else if s.opts.OnUpdate != nil {
		s.opts.OnUpdate(reading)
	}
	return result
}

// Run refresca periódicamente hasta que se cancele el contexto. Es el
// antiguo BackgroundMetricsWorker: primera lectura inmediata y luego una por
// intervalo.
func (s *Store) Run(ctx context.Context, interval time.Duration) {
	s.Refresh(ctx, true)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.Refresh(ctx, true)
		}
	}
}

func (s *Store) copyCurrent() *Snapshot {
	if s.current == nil {
		return nil
	}
	snap := *s.current
	return &snap
}
