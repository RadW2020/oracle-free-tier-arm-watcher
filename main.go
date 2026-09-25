// Package main es el punto de entrada de la aplicación: lee la
// configuración, elige la fuente de datos y levanta el servidor HTTP con los
// endpoints heredados, la API /v1 y el servidor MCP.
//
// La lógica vive en internal/: freetier (reglas de dominio), source (OCI y
// fixtures), snapshot (lectura guardada), api (operaciones), httpapi y
// mcpserver (transportes) y audit (clientes y auditoría).
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/RadW2020/oracle-free-tier-arm-watcher/internal/app"
	"github.com/RadW2020/oracle-free-tier-arm-watcher/internal/freetier"
	"github.com/RadW2020/oracle-free-tier-arm-watcher/internal/source"
	"github.com/joho/godotenv"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
)

// version se fija al compilar: -ldflags "-X main.version=...".
var version = "dev"

// logger es el logger estructurado global
var logger zerolog.Logger

// getEnv obtiene una variable de entorno con un valor por defecto
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getDuration(key string, def time.Duration) time.Duration {
	raw := os.Getenv(key)
	if raw == "" {
		return def
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		logger.Warn().Err(err).Str("var", key).Dur("default", def).Msg("Invalid duration, using default")
		return def
	}
	return d
}

// validateOCI avisa de las credenciales que faltan (no bloquea el inicio).
func validateOCI(oci *source.OCI) {
	if missing := oci.MissingConfig(); len(missing) > 0 {
		logger.Warn().
			Strs("missing_vars", missing).
			Msg("OCI credentials not fully configured - some endpoints will return NOT_CONFIGURED")
		return
	}
	keyPath := os.Getenv("OCI_PRIVATE_KEY_PATH")
	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		logger.Error().Str("path", keyPath).Msg("Private key file not found")
		return
	}
	logger.Info().Msg("OCI credentials validated successfully")
}

// main es el punto de entrada del programa
func main() {
	// Configurar logger estructurado
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	logger = zerolog.New(os.Stdout).With().Timestamp().Logger()

	// En desarrollo, usar output legible
	if os.Getenv("ENV") == "development" {
		logger = logger.Output(zerolog.ConsoleWriter{Out: os.Stdout})
	}

	// Cargar variables de entorno desde .env
	if err := godotenv.Load(); err != nil {
		logger.Info().Msg("No .env file found, using environment variables")
	}

	port := getEnv("PORT", "8088")
	interval := getDuration("METRICS_INTERVAL", 15*time.Minute)

	dataSource := getEnv("DATA_SOURCE", "oci")
	ociConfig := source.OCIConfigFromEnv()
	if dataSource == "oci" {
		validateOCI(source.NewOCI(ociConfig))
	} else {
		logger.Warn().Str("scenario", getEnv("FIXTURE_SCENARIO", "incident-2026-09-17")).Msg("DATA_SOURCE=fixture: serving a SYNTHETIC scenario, not a real tenancy")
	}

	watcher, err := app.New(app.Config{
		DataSource:      dataSource,
		Scenario:        getEnv("FIXTURE_SCENARIO", "incident-2026-09-17"),
		OCI:             ociConfig,
		APIKey:          os.Getenv("API_KEY"),
		APIClients:      os.Getenv("API_CLIENTS"),
		AuthDisabled:    os.Getenv("AUTH_MODE") == "disabled",
		RefreshInterval: interval,
		RefreshCooldown: getDuration("REFRESH_COOLDOWN", time.Minute),
		Version:         version,
		Logger:          logger,
		Metrics:         promhttp.Handler(),
		OnUpdate: func(r freetier.Reading) {
			updateMetrics(r)
			logger.Info().Bool("complete", r.Complete()).Msg("Metrics worker: Successfully updated Prometheus metrics")
		},
		OnError: func(err error) {
			ociReadsTotal.WithLabelValues("error").Inc()
			code, _ := source.Classify(err)
			logger.Error().Err(err).Str("error_code", code).Msg("Metrics worker: Error fetching OCI usage")
		},
	})
	if err != nil {
		logger.Fatal().Err(err).Msg("Invalid configuration")
	}
	registerSnapshotAge(watcher.Store)
	registry := watcher.Registry
	if !registry.Configured() && registry.Enforced() {
		logger.Warn().Msg("⚠️  No API_KEY or API_CLIENTS - legacy endpoints are public and /v1 and /mcp reject every request")
	} else if !registry.Enforced() {
		logger.Warn().Msg("AUTH_MODE=disabled: every endpoint is open (fixture demo only)")
	} else {
		logger.Info().Strs("clients", registry.Clients()).Msg("🔒 API authentication enabled")
	}
	src := watcher.Source

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Iniciar el worker de métricas en segundo plano
	logger.Info().Dur("interval", interval).Msg("Starting background metrics worker")
	go watcher.Run(ctx)

	// Sin WriteTimeout global: cada handler acota su propio contexto (las
	// llamadas a OCI llevan timeout), y un WriteTimeout cortaría respuestas
	// legítimas. Sí se acota la lectura de cabeceras, que es lo que un
	// cliente lento o malicioso puede alargar sin coste.
	server := &http.Server{
		Addr:              ":" + port,
		Handler:           watcher.Handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	logger.Info().
		Str("port", port).
		Str("data_source", src.Kind()).
		Str("version", version).
		Bool("auth_enforced", registry.Enforced()).
		Msg("🔍 Oracle Free Tier Watcher started")

	base := "http://localhost:" + port
	fmt.Println(strings.Join([]string{
		"📊 Usage endpoint: " + base + "/usage",
		"💚 Health check: " + base + "/health",
		"📋 Limits info: " + base + "/limits",
		"⚡ Quick status: " + base + "/status",
		"🤖 Agent API: " + base + "/v1/status  (OpenAPI: " + base + "/openapi.json)",
		"🔌 MCP server: " + base + "/mcp",
	}, "\n"))

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	// Iniciar el servidor
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Fatal().Err(err).Msg("Server stopped")
	}
	logger.Info().Msg("Server stopped")
}
