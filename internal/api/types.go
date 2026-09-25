package api

import (
	"time"

	"github.com/RadW2020/oracle-free-tier-arm-watcher/internal/audit"
	"github.com/RadW2020/oracle-free-tier-arm-watcher/internal/freetier"
	"github.com/RadW2020/oracle-free-tier-arm-watcher/internal/source"
)

// Los tipos de este archivo son el contrato de la API /v1 y del servidor
// MCP. Los esquemas JSON que ven los agentes (inputSchema/outputSchema de las
// tools y los componentes de /openapi.json) se generan de aquí, así que las
// descripciones de los campos están escritas para ellos.

// StatusValue es el estado de la cuota acumulativa.
type StatusValue string

// Resolution es la resolución de una serie temporal.
type Resolution string

const (
	Resolution1m Resolution = "1m"
	Resolution5m Resolution = "5m"
	Resolution1h Resolution = "1h"
)

// BillingVerdict es el veredicto de assess_billing_risk.
type BillingVerdict string

const (
	VerdictNoRisk  BillingVerdict = "NO_RISK"
	VerdictAtRisk  BillingVerdict = "AT_RISK"
	VerdictUnknown BillingVerdict = "UNKNOWN"
)

// HealthValue es la salud del watcher.
type HealthValue string

const (
	HealthHealthy  HealthValue = "healthy"
	HealthDegraded HealthValue = "degraded"
)

// Meta acompaña a cada respuesta.
type Meta struct {
	RequestID          string     `json:"requestId" jsonschema:"ID of this request; it appears in the watcher's audit log."`
	APIVersion         string     `json:"apiVersion"`
	DataSource         string     `json:"dataSource" jsonschema:"oci for the real tenancy; fixture for a synthetic demo scenario. Never present fixture numbers as real account data."`
	Scenario           string     `json:"scenario,omitempty" jsonschema:"Fixture scenario name, when dataSource is fixture."`
	Scope              string     `json:"scope" jsonschema:"What part of the tenancy is measured."`
	GeneratedAt        time.Time  `json:"generatedAt" jsonschema:"The watcher's current time. Use it (not your own clock) as now: fixture scenarios run on a frozen clock."`
	ObservedAt         *time.Time `json:"observedAt,omitempty" jsonschema:"When the underlying snapshot was read from OCI."`
	SnapshotAgeSeconds *int       `json:"snapshotAgeSeconds,omitempty" jsonschema:"Age of the snapshot. The watcher refreshes every 15 minutes by default."`
	Complete           bool       `json:"complete" jsonschema:"False when some data could not be read: treat missing values as unknown, never as zero."`
}

// Warning es un aviso con código estable.
type Warning struct {
	Code    string `json:"code" jsonschema:"quota_threshold, ingress_throttle_drops, source_unavailable or stale_snapshot."`
	Message string `json:"message"`
}

// Unavailable es un dato que no se pudo leer.
type Unavailable struct {
	Signal    string `json:"signal" jsonschema:"The data source or quota that could not be read."`
	ErrorCode string `json:"errorCode" jsonschema:"Classified failure: oci_throttled, oci_permission, oci_auth, oci_unavailable, timeout, network, not_configured..."`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}

// NextStep sugiere qué mirar después.
type NextStep struct {
	Tool      string         `json:"tool"`
	Reason    string         `json:"reason"`
	Arguments map[string]any `json:"arguments,omitempty" jsonschema:"Suggested arguments for the tool."`
}

// ---------------------------------------------------------------------------
// get_free_tier_status
// ---------------------------------------------------------------------------

// StatusRequest no tiene parámetros.
type StatusRequest struct{}

// Driver es una cuota acumulativa y lo cerca que está de su umbral.
type Driver struct {
	Quota         freetier.QuotaID `json:"quota"`
	Name          string           `json:"name"`
	Percentage    float64          `json:"percentage" jsonschema:"Percent of the free limit used (one decimal). Meaningless when available is false."`
	WarnThreshold int              `json:"warnThreshold" jsonschema:"Percent at which this quota raises a warning."`
	Available     bool             `json:"available"`
}

// SaturationSummary son los valores instantáneos de saturación.
type SaturationSummary struct {
	Available                    bool     `json:"available"`
	CPUPercent                   float64  `json:"cpuPercent"`
	MemoryPercent                float64  `json:"memoryPercent"`
	IngressMBPerMin              float64  `json:"ingressMBPerMin" jsonschema:"Real VNIC inbound traffic, MB per minute."`
	EgressMBPerMin               float64  `json:"egressMBPerMin"`
	IngressThrottleDropsLastHour float64  `json:"ingressThrottleDropsLastHour" jsonschema:"Packets dropped by the OCI ingress shaper in the last hour. > 0 means other services on the host were losing packets."`
	SampleAgeSeconds             int      `json:"sampleAgeSeconds" jsonschema:"Age of the newest datapoint; OCI Monitoring publishes a few minutes late."`
	MissingSignals               []string `json:"missingSignals" jsonschema:"Signals with no datapoints: their value above is 0 only because there is nothing else to put."`
}

// StatusResult es la respuesta de get_free_tier_status.
type StatusResult struct {
	Status               StatusValue       `json:"status" jsonschema:"OK / ATTENTION (>=60%) / WARNING (>=80%) / CRITICAL (>=90%) over the accruing quotas that can lead to a bill, or UNKNOWN when everything known is OK but an accruing quota could not be read."`
	Summary              string            `json:"summary" jsonschema:"One-sentence deterministic summary of the numbers below."`
	Complete             bool              `json:"complete"`
	Drivers              []Driver          `json:"drivers" jsonschema:"The accruing quotas (object storage, DB storage, egress), highest first."`
	AllocationPercentage int               `json:"allocationPercentage" jsonschema:"Highest allocation among OCPUs, RAM, block storage, IPs and DBs. 100% is the goal of a well-used free tier, not an incident."`
	Saturation           SaturationSummary `json:"saturation"`
	Warnings             []Warning         `json:"warnings"`
	Unavailable          []Unavailable     `json:"unavailable"`
	NextSteps            []NextStep        `json:"nextSteps"`
	Meta                 Meta              `json:"meta"`
}

// ---------------------------------------------------------------------------
// get_quota_usage
// ---------------------------------------------------------------------------

// QuotasRequest filtra las cuotas.
type QuotasRequest struct {
	Quota        freetier.QuotaID `json:"quota,omitempty" jsonschema:"Return only this quota."`
	NameContains string           `json:"nameContains,omitempty" jsonschema:"Return only resources whose name contains this text (case-insensitive), and only the quotas that have them. All matches are returned; if there are several, ask the user which one they mean."`
}

// Resource es un recurso detrás de una cuota.
type Resource struct {
	Kind      string   `json:"kind" jsonschema:"instance, boot_volume, block_volume, bucket or load_balancer."`
	Name      string   `json:"name"`
	Size      *float64 `json:"size" jsonschema:"Contribution to the quota in its unit; null when it could not be read."`
	Unit      string   `json:"unit"`
	SizeKnown bool     `json:"sizeKnown"`
	State     string   `json:"state,omitempty"`
	Detail    string   `json:"detail,omitempty"`
}

// QuotaView es una cuota con su uso.
type QuotaView struct {
	Quota         freetier.QuotaID `json:"quota"`
	Name          string           `json:"name"`
	Family        freetier.Family  `json:"family" jsonschema:"allocated (sized by design; 100% is normal) or accruing (grows on its own; over the limit is billed)."`
	Description   string           `json:"description"`
	Used          float64          `json:"used"`
	Limit         float64          `json:"limit"`
	Unit          string           `json:"unit" jsonschema:"ocpu, GB (2^30 bytes) or count."`
	Percentage    float64          `json:"percentage"`
	WarnThreshold int              `json:"warnThreshold,omitempty" jsonschema:"Accruing quotas only."`
	Available     bool             `json:"available" jsonschema:"False when the data could not be read; used/percentage are then not real values."`
	ErrorCode     string           `json:"errorCode,omitempty"`
	Resources     []Resource       `json:"resources"`
}

// QuotasResult es la respuesta de get_quota_usage.
type QuotasResult struct {
	Quotas  []QuotaView `json:"quotas"`
	Matches *int        `json:"matches,omitempty" jsonschema:"Number of resources matching nameContains, when it was given."`
	Notes   []string    `json:"notes"`
	Meta    Meta        `json:"meta"`
}

// ---------------------------------------------------------------------------
// assess_billing_risk
// ---------------------------------------------------------------------------

// BillingRequest no tiene parámetros.
type BillingRequest struct{}

// BillingQuota es una cuota acumulativa evaluada.
type BillingQuota struct {
	Quota               freetier.QuotaID `json:"quota"`
	Name                string           `json:"name"`
	Available           bool             `json:"available"`
	Used                float64          `json:"used"`
	Limit               float64          `json:"limit"`
	Unit                string           `json:"unit"`
	Percentage          float64          `json:"percentage"`
	WarnThreshold       int              `json:"warnThreshold"`
	Status              string           `json:"status" jsonschema:"ok, warning (over its threshold), projected_over (on track to exceed the free limit this month), over_limit, or unknown."`
	ProjectedMonthEnd   *float64         `json:"projectedMonthEnd,omitempty" jsonschema:"Egress only: linear projection to the end of the month, in GB."`
	ProjectedPercentage *float64         `json:"projectedPercentage,omitempty"`
	CrossesLimitOn      *string          `json:"crossesLimitOn,omitempty" jsonschema:"Egress only: UTC date (YYYY-MM-DD) on which the free limit would be crossed at the current rate."`
	Reason              string           `json:"reason"`
}

// BillingResult es la respuesta de assess_billing_risk.
type BillingResult struct {
	Verdict       BillingVerdict `json:"verdict" jsonschema:"AT_RISK if any accruing quota is over its threshold or projected over its limit; UNKNOWN if none is at risk but one could not be read; otherwise NO_RISK."`
	Month         string         `json:"month" jsonschema:"Calendar month evaluated (UTC), YYYY-MM."`
	DaysElapsed   float64        `json:"daysElapsed"`
	DaysRemaining float64        `json:"daysRemaining"`
	Quotas        []BillingQuota `json:"quotas"`
	Method        string         `json:"method"`
	NotCovered    []string       `json:"notCovered" jsonschema:"What this assessment cannot see. Do not claim certainty about these."`
	Meta          Meta           `json:"meta"`
}

// ---------------------------------------------------------------------------
// get_saturation_timeline
// ---------------------------------------------------------------------------

// SaturationRequest pide una línea de tiempo.
type SaturationRequest struct {
	Start         string          `json:"start" jsonschema:"Window start, RFC 3339 in UTC, e.g. 2026-09-17T15:00:00Z. Convert local times first (Europe/Madrid is UTC+2 in summer, UTC+1 in winter)."`
	End           string          `json:"end,omitempty" jsonschema:"Window end, RFC 3339 UTC. Defaults to now."`
	Resolution    Resolution      `json:"resolution,omitempty" jsonschema:"Bucket size. Defaults to 1m. Max window: 24h at 1m, 7d at 5m, 90d at 1h."`
	Metrics       []source.Metric `json:"metrics,omitempty" jsonschema:"Signals to return. Defaults to all five."`
	IncludePoints *bool           `json:"includePoints,omitempty" jsonschema:"Return the datapoints (default true). Set false to get only the summary, which allows larger windows."`
}

// Window es la ventana efectiva de una línea de tiempo.
type Window struct {
	Start      time.Time  `json:"start"`
	End        time.Time  `json:"end"`
	Resolution Resolution `json:"resolution"`
	Note       string     `json:"note,omitempty"`
}

// SeriesView es una serie con su procedencia.
type SeriesView struct {
	Metric      source.Metric  `json:"metric"`
	Unit        string         `json:"unit"`
	Description string         `json:"description"`
	Namespace   string         `json:"namespace" jsonschema:"OCI Monitoring namespace the data comes from."`
	Query       string         `json:"query" jsonschema:"MQL query that produced the series."`
	Aggregation string         `json:"aggregation" jsonschema:"sum (bytes, packets per bucket) or mean (percent)."`
	PointCount  int            `json:"pointCount"`
	Points      []source.Point `json:"points" jsonschema:"Datapoints, oldest first. t is the END of each bucket. Empty when includePoints is false."`
}

// MetricSummary resume una serie.
type MetricSummary struct {
	Metric source.Metric `json:"metric"`
	Unit   string        `json:"unit"`
	Points int           `json:"points"`
	Min    float64       `json:"min"`
	Max    float64       `json:"max"`
	MaxAt  *time.Time    `json:"maxAt,omitempty" jsonschema:"End of the bucket with the maximum value."`
	Mean   float64       `json:"mean"`
	Total  *float64      `json:"total,omitempty" jsonschema:"Sum over the window, for sum metrics."`
}

// DropInterval es un tramo continuo con paquetes descartados.
type DropInterval struct {
	Start         time.Time `json:"start" jsonschema:"Start of the first bucket with drops."`
	End           time.Time `json:"end" jsonschema:"End of the last bucket with drops."`
	TotalDrops    float64   `json:"totalDrops"`
	PeakPerBucket float64   `json:"peakPerBucket"`
}

// TimelineSummary resume la ventana.
type TimelineSummary struct {
	Metrics               []MetricSummary `json:"metrics"`
	ThrottleDropIntervals []DropInterval  `json:"throttleDropIntervals" jsonschema:"Contiguous stretches with ingress throttle drops > 0. Empty when there were none or the metric was not requested."`
	TotalThrottleDrops    *float64        `json:"totalThrottleDrops,omitempty"`
}

// DataLag describe el retraso de publicación.
type DataLag struct {
	NewestPointAt *time.Time `json:"newestPointAt,omitempty"`
	LagSeconds    *int       `json:"lagSeconds,omitempty" jsonschema:"How far behind now the newest datapoint is."`
	Note          string     `json:"note"`
}

// SaturationResult es la respuesta de get_saturation_timeline.
type SaturationResult struct {
	Window      Window          `json:"window"`
	Series      []SeriesView    `json:"series"`
	Summary     TimelineSummary `json:"summary"`
	DataLag     DataLag         `json:"dataLag"`
	BucketLabel string          `json:"bucketLabel" jsonschema:"end_of_interval: a 1m point at 15:15 covers 15:14:00-15:15:00."`
	Unavailable []Unavailable   `json:"unavailable"`
	Meta        Meta            `json:"meta"`
}

// ---------------------------------------------------------------------------
// get_watcher_diagnostics
// ---------------------------------------------------------------------------

// DiagnosticsRequest no tiene parámetros.
type DiagnosticsRequest struct{}

// ErrorInfo es un error resumido.
type ErrorInfo struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}

// SnapshotInfo describe la lectura guardada.
type SnapshotInfo struct {
	Present                bool       `json:"present"`
	ObservedAt             *time.Time `json:"observedAt,omitempty"`
	AgeSeconds             *int       `json:"ageSeconds,omitempty"`
	RefreshIntervalSeconds int        `json:"refreshIntervalSeconds"`
	LastAttemptAt          *time.Time `json:"lastAttemptAt,omitempty"`
	LastSuccessAt          *time.Time `json:"lastSuccessAt,omitempty"`
	LastError              *ErrorInfo `json:"lastError,omitempty"`
	FetchDurationMs        int64      `json:"fetchDurationMs"`
}

// WindowLimit es el tamaño máximo de ventana para una resolución.
type WindowLimit struct {
	Resolution     Resolution `json:"resolution"`
	MaxWindowHours int        `json:"maxWindowHours"`
}

// OperationalLimits son los límites que protegen a OCI.
type OperationalLimits struct {
	RefreshCooldownSeconds    int           `json:"refreshCooldownSeconds"`
	TimelineRequestsPerMinute int           `json:"timelineRequestsPerMinute"`
	TimelineWindows           []WindowLimit `json:"timelineWindows"`
	MaxPointsPerResponse      int           `json:"maxPointsPerResponse"`
	RetentionDays             int           `json:"retentionDays"`
}

// SourceView es el estado de una fuente de datos.
type SourceView struct {
	Name      string `json:"name"`
	Available bool   `json:"available"`
	ErrorCode string `json:"errorCode,omitempty"`
	Message   string `json:"message,omitempty"`
	Retryable bool   `json:"retryable"`
}

// DiagnosticsResult es la respuesta de get_watcher_diagnostics.
type DiagnosticsResult struct {
	Health       HealthValue       `json:"health"`
	Reasons      []string          `json:"reasons" jsonschema:"Why health is degraded. Empty when healthy."`
	Version      string            `json:"version"`
	DataSource   string            `json:"dataSource"`
	Configured   bool              `json:"configured" jsonschema:"Whether OCI credentials are configured."`
	AuthEnforced bool              `json:"authEnforced"`
	Client       string            `json:"client" jsonschema:"The client name this key authenticates as."`
	ClientScopes []string          `json:"clientScopes"`
	Snapshot     SnapshotInfo      `json:"snapshot"`
	Sources      []SourceView      `json:"sources"`
	Limits       OperationalLimits `json:"limits"`
	Meta         Meta              `json:"meta"`
}

// ---------------------------------------------------------------------------
// refresh_usage_snapshot
// ---------------------------------------------------------------------------

// RefreshRequest pide una lectura nueva.
type RefreshRequest struct {
	Reason string `json:"reason,omitempty" jsonschema:"Why a fresh read is needed. Recorded in the audit log."`
}

// RefreshResult es la respuesta de refresh_usage_snapshot.
type RefreshResult struct {
	Refreshed         bool          `json:"refreshed" jsonschema:"True if OCI was read (by this call or by one already running)."`
	Reason            string        `json:"reason" jsonschema:"refreshed, coalesced (joined a read already running) or cooldown (too soon: the existing snapshot is returned)."`
	RetryAfterSeconds int           `json:"retryAfterSeconds,omitempty"`
	DurationMs        int64         `json:"durationMs"`
	Complete          bool          `json:"complete"`
	Unavailable       []Unavailable `json:"unavailable"`
	Meta              Meta          `json:"meta"`
}

// ---------------------------------------------------------------------------
// audit (sólo REST)
// ---------------------------------------------------------------------------

// AuditRequest filtra el registro de auditoría.
type AuditRequest struct {
	Client    string `json:"client,omitempty"`
	Operation string `json:"operation,omitempty"`
	Outcome   string `json:"outcome,omitempty" jsonschema:"ok, error or denied."`
	Limit     int    `json:"limit,omitempty" jsonschema:"Max events, newest first. Default 50, max 500."`
}

// AuditResult es la respuesta de /v1/audit.
type AuditResult struct {
	Events []audit.Event `json:"events"`
	Meta   Meta          `json:"meta"`
}
