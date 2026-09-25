// Package httpapi sirve HTTP: los endpoints heredados, la API /v1, el
// documento OpenAPI y el punto de montaje del servidor MCP.
package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/RadW2020/oracle-free-tier-arm-watcher/internal/audit"
	"github.com/RadW2020/oracle-free-tier-arm-watcher/internal/freetier"
	"github.com/RadW2020/oracle-free-tier-arm-watcher/internal/source"
	"github.com/rs/zerolog"
)

// Los endpoints heredados (/usage, /status, /limits, /health) mantienen su
// JSON: los checks de Checkly asiertan sobre él y un campo que desaparece los
// rompe (ver el comentario de MaxUsagePercentage). Todo lo nuevo es aditivo,
// y los tests de contrato de este paquete lo vigilan.
//
// Siguen leyendo OCI en directo en cada petición, como siempre: así el check
// "Oracle Free Tier Monitor" prueba de punta a punta el camino watcher → OCI,
// y no sólo que el watcher tiene algo guardado. La API /v1 lee la lectura
// guardada.

// UsageResponse es la respuesta del endpoint /usage.
type UsageResponse struct {
	Status string `json:"status"`
	// Ver StatusResponse: maximo de la cuota acumulativa, no de toda.
	MaxUsagePercentage   int                     `json:"maxUsagePercentage"`
	AllocationPercentage int                     `json:"allocationPercentage"`
	Warnings             []string                `json:"warnings"`
	Timestamp            string                  `json:"timestamp"`
	Configured           bool                    `json:"configured"`
	Usage                *freetier.AllUsage      `json:"usage,omitempty"`
	FreeTierLimits       freetier.FreeTierLimits `json:"freeTierLimits"`
	// Complete y Sources son aditivos: dicen qué fuentes no respondieron,
	// porque sus valores de arriba son ceros de relleno, no mediciones.
	Complete bool                    `json:"complete"`
	Sources  []freetier.SourceStatus `json:"sources,omitempty"`
	Error    string                  `json:"error,omitempty"`
	Message  string                  `json:"message,omitempty"`
}

// HealthResponse es la respuesta del endpoint /health.
type HealthResponse struct {
	Status    string `json:"status"`
	Timestamp string `json:"timestamp"`
}

// StatusResponse es la respuesta del endpoint /status.
type StatusResponse struct {
	Status string `json:"status"`
	// MaxUsagePercentage es el maximo de la cuota que se llena sola, que es
	// la que marca el estado (ver freetier.Assess).
	//
	// SIN omitempty a proposito. Con el, un 0 desaparecia del JSON, y 0 es
	// justo el valor normal desde que el estado mide solo cuota acumulativa:
	// el check 'Oracle Free Tier Monitor' asierta $.maxUsagePercentage y
	// empezo a fallar con "Expected JSON array to satisfy comparison" —el
	// campo no existia— el 18/09/2026. Un campo que se evapora al valer cero
	// es una trampa para cualquiera que lo consuma.
	MaxUsagePercentage int `json:"maxUsagePercentage"`
	// AllocationPercentage es el maximo de la cuota asignada por diseno.
	// Estar al 100 % aqui es el objetivo de un Free Tier aprovechado, no una
	// incidencia: se informa, no escala.
	AllocationPercentage int      `json:"allocationPercentage"`
	Warnings             []string `json:"warnings,omitempty"`
	Timestamp            string   `json:"timestamp"`
	// Complete es aditivo, como en /usage.
	Complete *bool  `json:"complete,omitempty"`
	Message  string `json:"message,omitempty"`
}

// LimitsResponse es la respuesta del endpoint /limits.
type LimitsResponse struct {
	FreeTierLimits freetier.FreeTierLimits `json:"freeTierLimits"`
	Timestamp      string                  `json:"timestamp"`
}

// legacyWarnings junta los avisos que publica la API heredada: cuota,
// saturación y fuentes que no respondieron. /status y /usage usan esta misma
// función; antes el aviso de descartes sólo salía en /usage.
func legacyWarnings(r freetier.Reading, a freetier.Assessment) []string {
	warnings := append([]string{}, a.Warnings...)
	warnings = append(warnings, freetier.SaturationWarnings(r.Usage.Saturation)...)
	return append(warnings, freetier.UnavailableWarnings(r)...)
}

func writeJSON(w http.ResponseWriter, statusCode int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(data)
}

type legacy struct {
	src      source.Source
	registry *audit.Registry
	logger   zerolog.Logger
	timeout  time.Duration
}

func now() string { return time.Now().UTC().Format(time.RFC3339) }

// healthHandler maneja GET /health.
func (l *legacy) healthHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, HealthResponse{Status: "ok", Timestamp: now()})
}

// limitsHandler maneja GET /limits.
func (l *legacy) limitsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, LimitsResponse{FreeTierLimits: freetier.Limits, Timestamp: now()})
}

// usageHandler maneja GET /usage.
func (l *legacy) usageHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !l.src.Configured() {
		writeJSON(w, http.StatusOK, UsageResponse{
			Status:         "NOT_CONFIGURED",
			Configured:     false,
			Timestamp:      now(),
			Error:          "OCI not configured",
			Message:        "Please configure your OCI credentials in the .env file",
			FreeTierLimits: freetier.Limits,
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), l.timeout)
	defer cancel()
	reading, err := l.src.Read(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, UsageResponse{
			Status:         "ERROR",
			Configured:     true,
			Timestamp:      now(),
			Error:          err.Error(),
			FreeTierLimits: freetier.Limits,
		})
		return
	}

	assessment := freetier.Assess(reading)
	writeJSON(w, http.StatusOK, UsageResponse{
		Status:               assessment.Status,
		MaxUsagePercentage:   assessment.AccruingPercentage,
		AllocationPercentage: assessment.AllocationPercentage,
		Warnings:             legacyWarnings(reading, assessment),
		Timestamp:            now(),
		Configured:           true,
		Usage:                &reading.Usage,
		FreeTierLimits:       freetier.Limits,
		Complete:             reading.Complete(),
		Sources:              reading.Sources,
	})
}

// statusHandler maneja GET /status (versión simplificada).
func (l *legacy) statusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !l.src.Configured() {
		writeJSON(w, http.StatusServiceUnavailable, StatusResponse{
			Status:    "NOT_CONFIGURED",
			Timestamp: now(),
			Message:   "OCI credentials not configured",
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), l.timeout)
	defer cancel()
	reading, err := l.src.Read(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, StatusResponse{
			Status:    "ERROR",
			Timestamp: now(),
			Message:   err.Error(),
		})
		return
	}

	// Mismo criterio que /usage: el estado lo marca la cuota que se llena
	// sola, no la que esta asignada por diseno (ver freetier.Assess).
	assessment := freetier.Assess(reading)
	complete := reading.Complete()
	writeJSON(w, http.StatusOK, StatusResponse{
		Status:               assessment.Status,
		MaxUsagePercentage:   assessment.AccruingPercentage,
		AllocationPercentage: assessment.AllocationPercentage,
		Warnings:             legacyWarnings(reading, assessment),
		Timestamp:            now(),
		Complete:             &complete,
	})
}

// authMiddleware protege los endpoints heredados con una API Key.
//
// Conserva el comportamiento de siempre: si no hay ninguna clave
// configurada, deja pasar (y lo avisa en el log). Lo que cambia es que la
// comparación ya es en tiempo constante y que cualquier cliente de
// API_CLIENTS con permiso de lectura también entra.
func (l *legacy) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// El endpoint /health no requiere autenticación (para health checks)
		if r.URL.Path == "/health" {
			next.ServeHTTP(w, r)
			return
		}

		if !l.registry.Configured() {
			if l.registry.Enforced() {
				l.logger.Warn().Msg("API_KEY not set - endpoints are unprotected")
			}
			next.ServeHTTP(w, r)
			return
		}

		providedKey := keyFrom(r)
		if providedKey == "" {
			if _, ok := l.registry.Authenticate(""); ok {
				next.ServeHTTP(w, r)
				return
			}
			l.logger.Warn().Str("ip", r.RemoteAddr).Str("path", r.URL.Path).Msg("Unauthorized request - missing API key")
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Missing X-API-Key header"})
			return
		}

		client, ok := l.registry.Authenticate(providedKey)
		if !ok || !client.Has(audit.ScopeRead) {
			l.logger.Warn().Str("ip", r.RemoteAddr).Str("path", r.URL.Path).Msg("Unauthorized request - invalid API key")
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Invalid API key"})
			return
		}

		// API Key válida, continuar
		next.ServeHTTP(w, r)
	}
}
