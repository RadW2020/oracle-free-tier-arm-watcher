package audit

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/rs/zerolog"
)

// Event es una llamada a una operación de la API /v1 o del servidor MCP.
type Event struct {
	Time       time.Time      `json:"time"`
	RequestID  string         `json:"requestId"`
	Client     string         `json:"client"`
	Transport  string         `json:"transport"`
	Session    string         `json:"session,omitempty"`
	Operation  string         `json:"operation"`
	Args       map[string]any `json:"args,omitempty"`
	Outcome    string         `json:"outcome"`
	ErrorCode  string         `json:"errorCode,omitempty"`
	DurationMs int64          `json:"durationMs"`
	DataSource string         `json:"dataSource"`
	// Affected lista lo que la operación cambió. Sólo refresh cambia algo
	// (la lectura guardada del watcher); nada puede cambiar la tenancy.
	Affected []string `json:"affected"`
}

// Outcomes de un evento.
const (
	OutcomeOK     = "ok"
	OutcomeError  = "error"
	OutcomeDenied = "denied"
)

var (
	requestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "watcher_requests_total",
		Help: "Calls to the agent-facing operations (/v1 and MCP), by client, transport, operation and outcome",
	}, []string{"client", "transport", "operation", "outcome"})
	requestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "watcher_request_duration_seconds",
		Help:    "Duration of the agent-facing operations",
		Buckets: []float64{0.005, 0.02, 0.1, 0.5, 1, 2.5, 5, 10, 30},
	}, []string{"transport", "operation"})
)

// Log registra eventos en el log estructurado, en un buffer circular que se
// puede consultar por API y en métricas de Prometheus.
type Log struct {
	logger zerolog.Logger

	mu    sync.Mutex
	ring  []Event
	next  int
	count int
}

// NewLog crea un registro con capacidad para los últimos size eventos.
func NewLog(logger zerolog.Logger, size int) *Log {
	if size <= 0 {
		size = 500
	}
	return &Log{logger: logger, ring: make([]Event, size)}
}

// Record registra un evento.
func (l *Log) Record(e Event) {
	if e.Affected == nil {
		e.Affected = []string{}
	}
	l.mu.Lock()
	l.ring[l.next] = e
	l.next = (l.next + 1) % len(l.ring)
	if l.count < len(l.ring) {
		l.count++
	}
	l.mu.Unlock()

	requestsTotal.WithLabelValues(e.Client, e.Transport, e.Operation, e.Outcome).Inc()
	requestDuration.WithLabelValues(e.Transport, e.Operation).Observe(float64(e.DurationMs) / 1000)

	ev := l.logger.Info()
	if e.Outcome != OutcomeOK {
		ev = l.logger.Warn()
	}
	ev.Str("audit", "agent_operation").
		Str("request_id", e.RequestID).
		Str("client", e.Client).
		Str("transport", e.Transport).
		Str("session", e.Session).
		Str("operation", e.Operation).
		Interface("args", e.Args).
		Str("outcome", e.Outcome).
		Str("error_code", e.ErrorCode).
		Int64("duration_ms", e.DurationMs).
		Str("data_source", e.DataSource).
		Strs("affected", e.Affected).
		Msg("agent operation")
}

// Filter selecciona eventos del registro.
type Filter struct {
	Client    string
	Operation string
	Outcome   string
	Limit     int
}

// Query devuelve los eventos que cumplen el filtro, del más reciente al más
// antiguo.
func (l *Log) Query(f Filter) []Event {
	if f.Limit <= 0 {
		f.Limit = 50
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	out := []Event{}
	for i := 0; i < l.count && len(out) < f.Limit; i++ {
		idx := (l.next - 1 - i + len(l.ring)) % len(l.ring)
		e := l.ring[idx]
		if f.Client != "" && e.Client != f.Client {
			continue
		}
		if f.Operation != "" && e.Operation != f.Operation {
			continue
		}
		if f.Outcome != "" && e.Outcome != f.Outcome {
			continue
		}
		out = append(out, e)
	}
	return out
}

// NewRequestID genera un ID de petición.
func NewRequestID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "req_" + hex.EncodeToString(b)
}
