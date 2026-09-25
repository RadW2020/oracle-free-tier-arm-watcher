// Package app monta el watcher completo a partir de su configuración.
//
// Lo usan main, los tests de integración y el arnés de evals: los tres
// levantan exactamente el mismo servidor, así que una eval que pasa contra el
// escenario de fixtures está probando el mismo código que corre en producción.
package app

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/RadW2020/oracle-free-tier-arm-watcher/internal/api"
	"github.com/RadW2020/oracle-free-tier-arm-watcher/internal/audit"
	"github.com/RadW2020/oracle-free-tier-arm-watcher/internal/freetier"
	"github.com/RadW2020/oracle-free-tier-arm-watcher/internal/httpapi"
	"github.com/RadW2020/oracle-free-tier-arm-watcher/internal/mcpserver"
	"github.com/RadW2020/oracle-free-tier-arm-watcher/internal/snapshot"
	"github.com/RadW2020/oracle-free-tier-arm-watcher/internal/source"
	"github.com/rs/zerolog"
)

// Config es la configuración del watcher.
type Config struct {
	// Source es la fuente de datos. Si es nil se construye a partir de
	// DataSource y Scenario.
	Source     source.Source
	DataSource string // "oci" | "fixture"
	Scenario   string
	OCI        source.OCIConfig

	APIKey       string
	APIClients   string
	AuthDisabled bool

	RefreshInterval time.Duration
	RefreshCooldown time.Duration
	Version         string
	Logger          zerolog.Logger
	Metrics         http.Handler
	OnUpdate        func(freetier.Reading)
	OnError         func(error)
}

// App es el watcher montado.
type App struct {
	Handler  http.Handler
	Source   source.Source
	Store    *snapshot.Store
	Service  *api.Service
	Audit    *audit.Log
	Registry *audit.Registry
	interval time.Duration
}

// New monta el watcher.
func New(cfg Config) (*App, error) {
	if cfg.RefreshInterval == 0 {
		cfg.RefreshInterval = 15 * time.Minute
	}
	if cfg.Version == "" {
		cfg.Version = "dev"
	}

	src := cfg.Source
	if src == nil {
		switch cfg.DataSource {
		case "", "oci":
			src = source.NewOCI(cfg.OCI)
		case "fixture":
			sc, err := source.LoadScenario(cfg.Scenario)
			if err != nil {
				return nil, err
			}
			src = source.NewFixture(sc)
		default:
			return nil, errors.New("DATA_SOURCE must be oci or fixture")
		}
	}

	registry, err := audit.ParseClients(cfg.APIClients, cfg.APIKey)
	if err != nil {
		return nil, err
	}
	if cfg.AuthDisabled {
		// Abrirlo todo sólo tiene sentido con datos sintéticos: con la
		// tenancy real sería publicar el estado de la cuenta.
		if src.Kind() != "fixture" {
			return nil, errors.New("AUTH_MODE=disabled is only allowed with DATA_SOURCE=fixture")
		}
		registry.AllowAnonymous()
	}

	store := snapshot.New(src, snapshot.Options{Cooldown: cfg.RefreshCooldown, OnUpdate: cfg.OnUpdate, OnError: cfg.OnError})
	auditLog := audit.NewLog(cfg.Logger, 500)
	svc := api.New(api.Config{Store: store, Audit: auditLog, Registry: registry, Version: cfg.Version, RefreshInterval: cfg.RefreshInterval})

	handler := httpapi.NewMux(httpapi.Deps{
		Service:  svc,
		Source:   src,
		Registry: registry,
		Logger:   cfg.Logger,
		Metrics:  cfg.Metrics,
		MCP:      mcpserver.Handler(mcpserver.New(svc, cfg.Version), registry),
		Version:  cfg.Version,
	})

	return &App{Handler: handler, Source: src, Store: store, Service: svc, Audit: auditLog, Registry: registry, interval: cfg.RefreshInterval}, nil
}

// Run refresca la lectura guardada hasta que se cancele el contexto.
func (a *App) Run(ctx context.Context) { a.Store.Run(ctx, a.interval) }

// Prime hace la primera lectura de forma síncrona. Los tests y las evals la
// usan para no depender de cuándo arranca el worker.
func (a *App) Prime(ctx context.Context) error {
	return a.Store.Refresh(ctx, true).Err
}
