package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/RadW2020/oracle-free-tier-arm-watcher/internal/api"
	"github.com/RadW2020/oracle-free-tier-arm-watcher/internal/audit"
	"github.com/RadW2020/oracle-free-tier-arm-watcher/internal/freetier"
	"github.com/RadW2020/oracle-free-tier-arm-watcher/internal/source"
	"github.com/rs/zerolog"
)

// Deps son las dependencias del servidor HTTP.
type Deps struct {
	Service  *api.Service
	Source   source.Source
	Registry *audit.Registry
	Logger   zerolog.Logger
	// Metrics sirve /metrics (Prometheus). Queda público, como siempre: lo
	// rasca Alloy en localhost.
	Metrics http.Handler
	// MCP se monta en /mcp.
	MCP http.Handler
	// Timeout acota cada petición a los endpoints heredados y a /v1.
	Timeout time.Duration
	Version string
}

// NewMux construye el enrutador completo.
func NewMux(d Deps) http.Handler {
	if d.Timeout == 0 {
		d.Timeout = 90 * time.Second
	}
	mux := http.NewServeMux()

	l := &legacy{src: d.Source, registry: d.Registry, logger: d.Logger, timeout: d.Timeout}
	mux.HandleFunc("/health", l.authMiddleware(l.healthHandler))
	mux.HandleFunc("/limits", l.authMiddleware(l.limitsHandler))
	mux.HandleFunc("/usage", l.authMiddleware(l.usageHandler))
	mux.HandleFunc("/status", l.authMiddleware(l.statusHandler))
	if d.Metrics != nil {
		mux.Handle("/metrics", d.Metrics)
	}

	doc := OpenAPI(d.Version)
	mux.HandleFunc("/openapi.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		writeJSON(w, http.StatusOK, doc)
	})

	v := &v1{svc: d.Service, registry: d.Registry, timeout: d.Timeout}
	for _, op := range api.Operations {
		mux.Handle(op.Path, v.handle(op))
	}
	mux.Handle("/v1/", v.withRequest(func(w http.ResponseWriter, r *http.Request) {
		if client, ok := d.Registry.Authenticate(keyFrom(r)); ok {
			r = r.WithContext(audit.WithClient(r.Context(), client))
		}
		v.fail(w, r, "unknown_route", &api.Error{
			Code: api.CodeNotFound, Message: "no such endpoint: " + r.URL.Path,
			Hint: "GET /openapi.json lists every operation; valid paths: " + strings.Join(v1Paths(), ", "),
		})
	}))

	if d.MCP != nil {
		mux.Handle("/mcp", d.MCP)
	}
	return mux
}

func v1Paths() []string {
	paths := make([]string, 0, len(api.Operations))
	for _, op := range api.Operations {
		paths = append(paths, op.Method+" "+op.Path)
	}
	return paths
}

type v1 struct {
	svc      *api.Service
	registry *audit.Registry
	timeout  time.Duration
}

var requestIDPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

// withRequest asigna un ID de petición (el del cliente si es razonable) y lo
// devuelve en X-Request-Id: es el hilo que une una respuesta con su línea en
// el registro de auditoría.
func (v *v1) withRequest(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-Id")
		if !requestIDPattern.MatchString(id) {
			id = audit.NewRequestID()
		}
		w.Header().Set("X-Request-Id", id)
		ctx := audit.WithRequest(r.Context(), id, "rest", "")
		next(w, r.WithContext(ctx))
	}
}

// keyFrom lee la clave de Authorization: Bearer o de X-API-Key.
func keyFrom(r *http.Request) string {
	if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
	}
	return strings.TrimSpace(r.Header.Get("X-API-Key"))
}

func (v *v1) handle(op api.Operation) http.Handler {
	return v.withRequest(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != op.Method {
			w.Header().Set("Allow", op.Method)
			v.fail(w, r, op.Name, &api.Error{Code: api.CodeMethodNotAllowed, Message: r.Method + " is not allowed on " + op.Path, Hint: "use " + op.Method})
			return
		}
		// Sin clientes configurados, /v1 no se abre: al contrario que los
		// endpoints heredados, aquí no hay compatibilidad que conservar.
		client, ok := v.registry.Authenticate(keyFrom(r))
		if !ok {
			w.Header().Set("WWW-Authenticate", `Bearer realm="oci-watcher"`)
			hint := "send Authorization: Bearer <key> (or X-API-Key: <key>)"
			if !v.registry.Configured() {
				hint = "no clients are configured: set API_CLIENTS (name:scope+scope:key) or API_KEY on the watcher"
			}
			v.fail(w, r, op.Name, &api.Error{Code: api.CodeUnauthenticated, Message: "missing or invalid API key", Hint: hint})
			return
		}
		ctx, cancel := context.WithTimeout(audit.WithClient(r.Context(), client), v.timeout)
		defer cancel()
		r = r.WithContext(ctx)

		out, err := v.dispatch(r, op)
		if err != nil {
			v.fail(w, r, "", api.AsError(err))
			return
		}
		writeJSON(w, http.StatusOK, out)
	})
}

// fail escribe un error. Si la petición no llegó a la operación (clave,
// método, ruta), deja además el evento de auditoría; si llegó, la operación
// ya lo dejó.
func (v *v1) fail(w http.ResponseWriter, r *http.Request, rejectedOp string, e *api.Error) {
	if rejectedOp != "" {
		v.svc.RecordRejected(r.Context(), rejectedOp, e.Code)
	}
	if e.RetryAfterSeconds > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(e.RetryAfterSeconds))
	}
	writeJSON(w, e.HTTPStatus(), api.ErrorEnvelope{Error: e})
}

func (v *v1) dispatch(r *http.Request, op api.Operation) (any, error) {
	ctx := r.Context()
	q := r.URL.Query()
	switch op.Name {
	case api.OpStatus:
		if err := onlyParams(q); err != nil {
			return nil, err
		}
		return v.svc.Status(ctx, api.StatusRequest{})
	case api.OpQuotas:
		if err := onlyParams(q, "quota", "nameContains"); err != nil {
			return nil, err
		}
		return v.svc.Quotas(ctx, api.QuotasRequest{Quota: freetier.QuotaID(q.Get("quota")), NameContains: q.Get("nameContains")})
	case api.OpBilling:
		if err := onlyParams(q); err != nil {
			return nil, err
		}
		return v.svc.BillingRisk(ctx, api.BillingRequest{})
	case api.OpSaturation:
		if err := onlyParams(q, "start", "end", "resolution", "metrics", "includePoints"); err != nil {
			return nil, err
		}
		req := api.SaturationRequest{Start: q.Get("start"), End: q.Get("end"), Resolution: api.Resolution(q.Get("resolution"))}
		for _, raw := range q["metrics"] {
			for _, m := range strings.Split(raw, ",") {
				if m = strings.TrimSpace(m); m != "" {
					req.Metrics = append(req.Metrics, source.Metric(m))
				}
			}
		}
		if raw := q.Get("includePoints"); raw != "" {
			b, err := strconv.ParseBool(raw)
			if err != nil {
				return nil, &api.Error{Code: api.CodeInvalidArgument, Field: "includePoints", Message: "includePoints must be true or false"}
			}
			req.IncludePoints = &b
		}
		return v.svc.Saturation(ctx, req)
	case api.OpDiagnostics:
		if err := onlyParams(q); err != nil {
			return nil, err
		}
		return v.svc.Diagnostics(ctx, api.DiagnosticsRequest{})
	case api.OpRefresh:
		var req api.RefreshRequest
		if err := decodeBody(r, &req); err != nil {
			return nil, err
		}
		return v.svc.Refresh(ctx, req)
	case api.OpAudit:
		if err := onlyParams(q, "client", "operation", "outcome", "limit"); err != nil {
			return nil, err
		}
		req := api.AuditRequest{Client: q.Get("client"), Operation: q.Get("operation"), Outcome: q.Get("outcome")}
		if raw := q.Get("limit"); raw != "" {
			n, err := strconv.Atoi(raw)
			if err != nil {
				return nil, &api.Error{Code: api.CodeInvalidArgument, Field: "limit", Message: "limit must be an integer"}
			}
			req.Limit = n
		}
		return v.svc.AuditLog(ctx, req)
	}
	return nil, &api.Error{Code: api.CodeInternal, Message: "operation not wired: " + op.Name}
}

// onlyParams rechaza parámetros desconocidos. Un `nameContain` mal escrito
// que se ignorase en silencio devolvería todo y el agente creería que ha
// filtrado.
func onlyParams(q url.Values, allowed ...string) error {
	ok := map[string]bool{}
	for _, a := range allowed {
		ok[a] = true
	}
	var unknown []string
	for k := range q {
		if !ok[k] {
			unknown = append(unknown, k)
		}
	}
	if len(unknown) == 0 {
		return nil
	}
	sort.Strings(unknown)
	valid := "none"
	if len(allowed) > 0 {
		valid = strings.Join(allowed, ", ")
	}
	return &api.Error{Code: api.CodeInvalidArgument, Field: unknown[0], Message: "unknown query parameter(s): " + strings.Join(unknown, ", "), Hint: "valid parameters: " + valid}
}

func decodeBody(r *http.Request, into any) error {
	body, err := io.ReadAll(io.LimitReader(r.Body, 4096))
	if err != nil {
		return &api.Error{Code: api.CodeInvalidArgument, Message: "cannot read request body"}
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		return nil
	}
	dec := json.NewDecoder(strings.NewReader(string(body)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(into); err != nil {
		return &api.Error{Code: api.CodeInvalidArgument, Message: "invalid JSON body: " + err.Error()}
	}
	return nil
}
