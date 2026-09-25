// Package mcpserver expone las operaciones de internal/api como tools MCP.
//
// Vive en el mismo proceso que el watcher y se sirve en /mcp (streamable
// HTTP). Es la razón de que un agente no necesite nunca la clave de firma de
// OCI: le basta una clave del watcher con los permisos justos, y el watcher
// —que ya tiene las credenciales— hace las llamadas.
package mcpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/RadW2020/oracle-free-tier-arm-watcher/internal/api"
	"github.com/RadW2020/oracle-free-tier-arm-watcher/internal/audit"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Instructions es lo que un agente lee al conectarse, antes de llamar a
// nada. Son las reglas de interpretación que no se deducen de un número.
const Instructions = `Oracle Free Tier Watcher: a read-only view of one Oracle Cloud Always Free tenancy (one ARM VM hosting several services).

How to read it:
- Two kinds of quota. Allocated quota (4 OCPUs, 24 GB RAM, 200 GB disk, IPs, DBs) is sized on purpose: 100% is normal and never a problem. Accruing quota (object storage, DB storage, monthly egress) grows on its own and going over the free limit is billed; only this drives status.
- UNKNOWN, available=false, sizeKnown=false or complete=false mean the value could not be read. Never report those as 0 and never fill them in: say they are unknown.
- Saturation (VNIC ingress, OCI ingress-throttle drops, CPU, memory) explains slowness, not bills. Throttle drops > 0 mean OCI was discarding inbound packets for every service on the host, which surfaces elsewhere as TCP connect timeouts.
- All times are UTC. The operator is in Europe/Madrid (CEST = UTC+2 in summer): convert before querying. Timeline points are labelled with the END of their bucket, and OCI Monitoring publishes a few minutes late.
- meta.generatedAt is the watcher's "now". Compute ages and "this month" from it and from meta.snapshotAgeSeconds, not from your own date: fixture scenarios (meta.dataSource=fixture, synthetic, not the real account) run on a clock frozen at the scenario's time.

These tools cannot change anything in the tenancy. refresh_usage_snapshot only re-reads OCI into the watcher's cache and needs the refresh scope. If asked to delete, stop or resize resources, explain that this interface cannot do that.

Start with get_free_tier_status and follow its nextSteps.`

// New crea el servidor MCP con las tools del catálogo.
func New(svc *api.Service, version string) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "oci-free-tier-watcher", Title: "Oracle Free Tier Watcher", Version: version}, &mcp.ServerOptions{
		Instructions: Instructions,
	})
	t := &tools{svc: svc}
	register(server, t, api.OpStatus, svc.Status)
	register(server, t, api.OpQuotas, svc.Quotas)
	register(server, t, api.OpBilling, svc.BillingRisk)
	register(server, t, api.OpSaturation, svc.Saturation)
	register(server, t, api.OpDiagnostics, svc.Diagnostics)
	register(server, t, api.OpRefresh, svc.Refresh)
	return server
}

type tools struct {
	svc *api.Service
}

// register añade una tool a partir de su entrada en el catálogo.
//
// La salida se declara con su esquema, pero la tool se registra con salida
// `any`: así, cuando la operación falla, el resultado lleva el sobre de error
// como texto con isError=true en vez de un objeto vacío que pasara por un
// resultado válido.
func register[In, Out any](server *mcp.Server, t *tools, name string, op func(context.Context, In) (Out, error)) {
	spec, ok := api.OperationByName(name)
	if !ok || !spec.MCP {
		panic("mcpserver: operation not in catalog: " + name)
	}
	in, err := api.SchemaFor(spec.Input)
	if err != nil {
		panic(err)
	}
	out, err := api.SchemaFor(spec.Output)
	if err != nil {
		panic(err)
	}
	destructive := false
	openWorld := spec.OpenWorld
	tool := &mcp.Tool{
		Name:         spec.Name,
		Title:        spec.Title,
		Description:  spec.Description,
		InputSchema:  in,
		OutputSchema: out,
		Annotations: &mcp.ToolAnnotations{
			Title:           spec.Title,
			ReadOnlyHint:    spec.ReadOnly,
			IdempotentHint:  spec.Idempotent,
			DestructiveHint: &destructive,
			OpenWorldHint:   &openWorld,
		},
	}
	mcp.AddTool(server, tool, func(ctx context.Context, req *mcp.CallToolRequest, input In) (*mcp.CallToolResult, any, error) {
		ctx, authErr := t.authenticate(ctx, req, name)
		if authErr != nil {
			return errorResult(authErr), nil, nil
		}
		result, err := op(ctx, input)
		if err != nil {
			return errorResult(api.AsError(err)), nil, nil
		}
		return nil, result, nil
	})
}

// authenticate resuelve el cliente de la llamada a partir de las cabeceras
// HTTP que la trajeron. El middleware HTTP ya rechazó las peticiones sin
// clave válida; esto identifica al cliente para los permisos y la auditoría.
func (t *tools) authenticate(ctx context.Context, req *mcp.CallToolRequest, op string) (context.Context, *api.Error) {
	var key string
	if req.Extra != nil && req.Extra.Header != nil {
		key = headerKey(req.Extra.Header)
	}
	session := ""
	if req.Session != nil {
		session = req.Session.ID()
	}
	ctx = audit.WithRequest(ctx, audit.NewRequestID(), "mcp", session)
	client, ok := t.svc.Registry().Authenticate(key)
	if !ok {
		t.svc.RecordRejected(ctx, op, api.CodeUnauthenticated)
		return ctx, &api.Error{Code: api.CodeUnauthenticated, Message: "missing or invalid API key", Hint: "configure the MCP client with Authorization: Bearer <key>"}
	}
	return audit.WithClient(ctx, client), nil
}

func errorResult(e *api.Error) *mcp.CallToolResult {
	raw, _ := json.Marshal(api.ErrorEnvelope{Error: e})
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: string(raw)}}}
}

func headerKey(h http.Header) string {
	if auth := h.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
	}
	return strings.TrimSpace(h.Get("X-API-Key"))
}

// Handler sirve el servidor MCP por streamable HTTP.
//
// Sin estado (Stateless) y con respuestas JSON: cada llamada es una petición
// HTTP independiente, que es lo que encaja con un servicio detrás de Traefik
// y con tools que no necesitan hablarle al cliente por su cuenta. Las
// peticiones sin clave válida se rechazan con 401 antes de llegar al
// protocolo, así que un cliente sin permisos ni siquiera ve la lista de tools.
func Handler(server *mcp.Server, registry *audit.Registry) http.Handler {
	h := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{
		Stateless:    true,
		JSONResponse: true,
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := registry.Authenticate(headerKey(r.Header)); !ok {
			w.Header().Set("WWW-Authenticate", `Bearer realm="oci-watcher"`)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(api.ErrorEnvelope{Error: &api.Error{
				Code: api.CodeUnauthenticated, Message: "missing or invalid API key",
				Hint: "configure the MCP client with the header Authorization: Bearer <key>",
			}})
			return
		}
		h.ServeHTTP(w, r)
	})
}
