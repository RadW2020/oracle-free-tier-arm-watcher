package api

import (
	"reflect"
	"time"

	"github.com/RadW2020/oracle-free-tier-arm-watcher/internal/audit"
	"github.com/RadW2020/oracle-free-tier-arm-watcher/internal/freetier"
	"github.com/RadW2020/oracle-free-tier-arm-watcher/internal/source"
	"github.com/google/jsonschema-go/jsonschema"
)

// Operation describe una operación una sola vez para todos los transportes:
// el servidor MCP toma de aquí el nombre, la descripción y las anotaciones de
// cada tool, y /openapi.json la ruta, el método y los esquemas.
type Operation struct {
	Name        string
	Title       string
	Description string
	Method      string
	Path        string
	Scope       audit.Scope
	ReadOnly    bool
	Idempotent  bool
	// OpenWorld es true cuando la operación llama a OCI en directo en vez
	// de leer la lectura guardada.
	OpenWorld bool
	// MCP es false para lo que no es para agentes (el registro de auditoría
	// es para el humano que quiere saber qué hizo el agente).
	MCP    bool
	Input  reflect.Type
	Output reflect.Type
}

// Operations es el catálogo, en el orden en que se listan.
var Operations = []Operation{
	{
		Name:  OpStatus,
		Title: "Free tier status",
		Description: "Start here. Overall state of the Oracle Cloud Always Free tenancy: whether a quota that can lead to a bill is close to its limit, " +
			"plus current host saturation and data completeness. `status` only considers accruing quotas (object storage, DB storage, monthly egress); " +
			"OCPUs, RAM and disk allocated at 100% are sized that way on purpose and never raise it. `status: UNKNOWN` means an accruing quota could not be read, " +
			"which is not the same as 0%. Follow `nextSteps` for what to check next. Reads the snapshot cached by the watcher (refreshed every 15 minutes); it does not call OCI.",
		Method: "GET", Path: "/v1/status", Scope: audit.ScopeRead, ReadOnly: true, Idempotent: true, MCP: true,
		Input: reflect.TypeFor[StatusRequest](), Output: reflect.TypeFor[StatusResult](),
	},
	{
		Name:  OpQuotas,
		Title: "Quota usage by resource",
		Description: "Usage of each Always Free quota against its limit, with the resources behind each number (instances, boot/block volumes, buckets, load balancers). " +
			"Use it for 'what is using X' or 'is there room for Y'. Filter with `quota` for one quota, or `nameContains` to find resources by name " +
			"(case-insensitive; every match is returned, so if there are several, ask which one the user means instead of picking). " +
			"A resource whose size could not be read has sizeKnown=false and size=null: it is not 0. Memory in use inside the VM is not a quota: see memory_percent in get_saturation_timeline. " +
			"Only one compartment is measured (meta.scope). Reads the cached snapshot.",
		Method: "GET", Path: "/v1/quotas", Scope: audit.ScopeRead, ReadOnly: true, Idempotent: true, MCP: true,
		Input: reflect.TypeFor[QuotasRequest](), Output: reflect.TypeFor[QuotasResult](),
	},
	{
		Name:  OpBilling,
		Title: "Billing risk this month",
		Description: "Will anything be billed this month? Evaluates the accruing quotas (object storage, DB storage, month-to-date egress) against their warning thresholds and free limits, " +
			"and projects egress linearly to the end of the calendar month (UTC), with the date it would cross the limit. " +
			"Verdict: NO_RISK, AT_RISK, or UNKNOWN when a needed value could not be read. The quota `status` from get_free_tier_status can be OK while this says AT_RISK: " +
			"that is the projection seeing what the current level does not. It cannot see OCI budget alerts, the invoice or paid services (`notCovered`): do not claim more than it checks. Reads the cached snapshot.",
		Method: "GET", Path: "/v1/billing-risk", Scope: audit.ScopeRead, ReadOnly: true, Idempotent: true, MCP: true,
		Input: reflect.TypeFor[BillingRequest](), Output: reflect.TypeFor[BillingResult](),
	},
	{
		Name:  OpSaturation,
		Title: "Host saturation timeline",
		Description: "Time series from OCI Monitoring for a past or current window: real VNIC ingress/egress bytes, packets dropped by the OCI ingress shaper, CPU and memory of the instance. " +
			"Use it to investigate slowness or timeouts ('why did the checks fail at 17:14?'). All times are UTC: convert local times first (the operator is in Europe/Madrid, CEST = UTC+2). " +
			"Each point is labelled with the END of its bucket. OCI publishes a few minutes late, so the newest minutes may be missing (`dataLag`). " +
			"`summary` has min/max/mean per metric and the stretches with throttle drops; drops > 0 mean every service on the host was losing packets, including TCP SYNs, which shows up elsewhere as connection timeouts. " +
			"Limits: window up to 24h at 1m, 7d at 5m, 90d at 1h; start within the last 90 days; at most 720 points in total when includePoints is true " +
			"(ask for fewer metrics, a coarser resolution, or includePoints=false for the summary only). Calls OCI live and is rate-limited per client.",
		Method: "GET", Path: "/v1/saturation", Scope: audit.ScopeRead, ReadOnly: true, Idempotent: true, OpenWorld: true, MCP: true,
		Input: reflect.TypeFor[SaturationRequest](), Output: reflect.TypeFor[SaturationResult](),
	},
	{
		Name:  OpDiagnostics,
		Title: "Watcher diagnostics",
		Description: "Health of the watcher itself: whether OCI credentials are configured, how old the cached snapshot is and when it was last refreshed, which data sources failed and why " +
			"(oci_throttled, oci_permission, oci_auth, timeout...) and whether retrying can help, whether auth is enforced, which scopes your key has, and the operational limits. " +
			"Use it when data looks missing or stale, or when monitoring reports the watcher as failing.",
		Method: "GET", Path: "/v1/diagnostics", Scope: audit.ScopeRead, ReadOnly: true, Idempotent: true, MCP: true,
		Input: reflect.TypeFor[DiagnosticsRequest](), Output: reflect.TypeFor[DiagnosticsResult](),
	},
	{
		Name:  OpRefresh,
		Title: "Refresh the usage snapshot",
		Description: "Ask the watcher to read OCI again now instead of waiting for its next 15-minute refresh. It only replaces the watcher's cached snapshot: it cannot change anything in the tenancy. " +
			"It costs 13+ OCI API calls, so it is guarded: within the cooldown it returns the existing snapshot with refreshed=false, reason=cooldown and retryAfterSeconds; " +
			"concurrent calls share one read (reason=coalesced). Requires the `refresh` scope. Use it only when the snapshot is stale or a source failed with a retryable error, then call get_free_tier_status again.",
		Method: "POST", Path: "/v1/snapshot/refresh", Scope: audit.ScopeRefresh, ReadOnly: false, Idempotent: true, OpenWorld: true, MCP: true,
		Input: reflect.TypeFor[RefreshRequest](), Output: reflect.TypeFor[RefreshResult](),
	},
	{
		Name:        OpAudit,
		Title:       "Audit log",
		Description: "What did each client do? The most recent calls to the /v1 API and the MCP tools, newest first, with client, operation, arguments, outcome, error code, duration and what they changed. Kept in memory (last 500 calls); the same events are in the JSON logs.",
		Method:      "GET", Path: "/v1/audit", Scope: audit.ScopeAudit, ReadOnly: true, Idempotent: true, MCP: false,
		Input: reflect.TypeFor[AuditRequest](), Output: reflect.TypeFor[AuditResult](),
	},
}

// OperationByName busca una operación del catálogo.
func OperationByName(name string) (Operation, bool) {
	for _, op := range Operations {
		if op.Name == name {
			return op, true
		}
	}
	return Operation{}, false
}

func enumSchema[T ~string](values ...T) *jsonschema.Schema {
	s := &jsonschema.Schema{Type: "string"}
	for _, v := range values {
		s.Enum = append(s.Enum, string(v))
	}
	return s
}

// typeSchemas son los esquemas de los tipos con valores cerrados: así un
// agente descubre los valores válidos en el propio esquema de la tool, sin
// tener que provocar un error para averiguarlos.
func typeSchemas() map[reflect.Type]*jsonschema.Schema {
	quotaIDs := make([]freetier.QuotaID, len(freetier.Quotas))
	for i, q := range freetier.Quotas {
		quotaIDs[i] = q.ID
	}
	metrics := make([]source.Metric, len(source.Metrics))
	for i, m := range source.Metrics {
		metrics[i] = m.Metric
	}
	return map[reflect.Type]*jsonschema.Schema{
		reflect.TypeFor[time.Time]():        {Type: "string", Format: "date-time"},
		reflect.TypeFor[StatusValue]():      enumSchema[StatusValue](freetier.StatusOK, freetier.StatusAttention, freetier.StatusWarning, freetier.StatusCritical, freetier.StatusUnknown),
		reflect.TypeFor[Resolution]():       enumSchema(Resolution1m, Resolution5m, Resolution1h),
		reflect.TypeFor[BillingVerdict]():   enumSchema(VerdictNoRisk, VerdictAtRisk, VerdictUnknown),
		reflect.TypeFor[HealthValue]():      enumSchema(HealthHealthy, HealthDegraded),
		reflect.TypeFor[freetier.QuotaID](): enumSchema(quotaIDs...),
		reflect.TypeFor[freetier.Family]():  enumSchema(freetier.FamilyAllocated, freetier.FamilyAccruing),
		reflect.TypeFor[source.Metric]():    enumSchema(metrics...),
		reflect.TypeFor[ErrorCode]():        enumSchema(ErrorCodes...),
	}
}

// SchemaFor genera el esquema JSON de un tipo del contrato.
func SchemaFor(t reflect.Type) (*jsonschema.Schema, error) {
	return jsonschema.ForType(t, &jsonschema.ForOptions{TypeSchemas: typeSchemas()})
}
