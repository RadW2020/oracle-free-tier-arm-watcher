package api

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/RadW2020/oracle-free-tier-arm-watcher/internal/audit"
	"github.com/RadW2020/oracle-free-tier-arm-watcher/internal/freetier"
	"github.com/RadW2020/oracle-free-tier-arm-watcher/internal/snapshot"
	"github.com/RadW2020/oracle-free-tier-arm-watcher/internal/source"
)

// Nombres de las operaciones. Son a la vez los nombres de las tools MCP y
// los operationId de /openapi.json.
const (
	OpStatus      = "get_free_tier_status"
	OpQuotas      = "get_quota_usage"
	OpBilling     = "assess_billing_risk"
	OpSaturation  = "get_saturation_timeline"
	OpDiagnostics = "get_watcher_diagnostics"
	OpRefresh     = "refresh_usage_snapshot"
	OpAudit       = "get_audit_log"
)

// ---------------------------------------------------------------------------
// get_free_tier_status
// ---------------------------------------------------------------------------

// Status es la vista de conjunto.
func (s *Service) Status(ctx context.Context, _ StatusRequest) (*StatusResult, error) {
	var out *StatusResult
	err := s.call(ctx, OpStatus, audit.ScopeRead, nil, func() ([]string, error) {
		snap, err := s.currentSnapshot()
		if err != nil {
			return nil, err
		}
		out = s.buildStatus(ctx, snap)
		return nil, nil
	})
	return out, err
}

func (s *Service) buildStatus(ctx context.Context, snap snapshot.Snapshot) *StatusResult {
	r := snap.Reading
	a := freetier.Assess(r)
	u := &r.Usage

	res := &StatusResult{
		Status:               StatusValue(a.Verdict),
		Complete:             r.Complete(),
		Drivers:              []Driver{},
		AllocationPercentage: a.AllocationPercentage,
		Warnings:             []Warning{},
		Unavailable:          unavailableOf(r),
		NextSteps:            []NextStep{},
		Meta:                 s.meta(ctx, &snap, r.Complete()),
	}

	for _, q := range freetier.AccruingQuotas() {
		d := Driver{Quota: q.ID, Name: q.Name, WarnThreshold: q.WarnThreshold, Available: r.Available(q.Source)}
		if d.Available {
			d.Percentage = q.Percentage(u)
			if int(d.Percentage) >= q.WarnThreshold {
				res.Warnings = append(res.Warnings, Warning{Code: "quota_threshold", Message: fmt.Sprintf("%s at %.1f%% of its free limit (warns at %d%%)", q.Name, d.Percentage, q.WarnThreshold)})
			}
		}
		res.Drivers = append(res.Drivers, d)
	}
	sort.SliceStable(res.Drivers, func(i, j int) bool { return res.Drivers[i].Percentage > res.Drivers[j].Percentage })

	sat := u.Saturation
	res.Saturation = SaturationSummary{
		Available:                    r.Available(freetier.SourceSaturation),
		CPUPercent:                   round1(sat.CPUPercentage),
		MemoryPercent:                round1(sat.MemoryPercentage),
		IngressMBPerMin:              round1(sat.IngressMBPerMin),
		EgressMBPerMin:               round1(sat.EgressMBPerMin),
		IngressThrottleDropsLastHour: sat.IngressDropsLastHour,
		SampleAgeSeconds:             sat.SampleAgeSeconds,
		MissingSignals:               nonNil(sat.MissingSignals),
	}
	now := s.src.Now()
	if sat.IngressDropsLastHour > 0 {
		res.Warnings = append(res.Warnings, Warning{Code: "ingress_throttle_drops", Message: freetier.SaturationWarnings(sat)[0]})
		res.NextSteps = append(res.NextSteps, NextStep{
			Tool:   OpSaturation,
			Reason: "the OCI ingress shaper dropped packets in the last hour; find when and how much",
			Arguments: map[string]any{
				"start":   now.Add(-time.Hour).Format(time.RFC3339),
				"metrics": []string{string(source.MetricIngressBytes), string(source.MetricIngressDrops), string(source.MetricCPU)},
			},
		})
	}
	for _, un := range res.Unavailable {
		res.Warnings = append(res.Warnings, Warning{Code: "source_unavailable", Message: fmt.Sprintf("%s could not be read (%s): its values are unknown, not zero", un.Signal, un.ErrorCode)})
	}
	if len(res.Unavailable) > 0 {
		res.NextSteps = append(res.NextSteps, NextStep{Tool: OpDiagnostics, Reason: "some data sources failed; see which and whether retrying helps"})
	}
	if s.stale(snap) {
		res.Warnings = append(res.Warnings, Warning{Code: "stale_snapshot", Message: fmt.Sprintf("the snapshot is %s old; the watcher normally refreshes every %s", now.Sub(r.ObservedAt).Round(time.Second), s.cfg.RefreshInterval)})
	}
	if s.stale(snap) || anyRetryable(res.Unavailable) {
		res.NextSteps = append(res.NextSteps, NextStep{Tool: OpRefresh, Reason: "the data is stale or failed transiently; a fresh read may recover it (needs the refresh scope)"})
	}
	if a.AccruingPercentage >= 50 || !a.Complete() {
		res.NextSteps = append(res.NextSteps, NextStep{Tool: OpBilling, Reason: "check whether an accruing quota will cross its free limit this month"})
	}

	res.Summary = statusSummary(res)
	return res
}

func statusSummary(res *StatusResult) string {
	var parts []string
	for _, d := range res.Drivers {
		if d.Available {
			parts = append(parts, fmt.Sprintf("%s %.1f%%", d.Name, d.Percentage))
		} else {
			parts = append(parts, d.Name+" unknown")
		}
	}
	sentence := fmt.Sprintf("Status %s. Accruing quotas: %s. Allocated quota at %d%% (by design).", res.Status, strings.Join(parts, ", "), res.AllocationPercentage)
	if res.Saturation.IngressThrottleDropsLastHour > 0 {
		sentence += fmt.Sprintf(" OCI dropped %.0f inbound packets in the last hour.", res.Saturation.IngressThrottleDropsLastHour)
	}
	return sentence
}

// ---------------------------------------------------------------------------
// get_quota_usage
// ---------------------------------------------------------------------------

// Quotas devuelve el uso por cuota con sus recursos.
func (s *Service) Quotas(ctx context.Context, req QuotasRequest) (*QuotasResult, error) {
	var out *QuotasResult
	args := map[string]any{}
	if req.Quota != "" {
		args["quota"] = req.Quota
	}
	if req.NameContains != "" {
		args["nameContains"] = req.NameContains
	}
	err := s.call(ctx, OpQuotas, audit.ScopeRead, args, func() ([]string, error) {
		if req.Quota != "" {
			if _, ok := freetier.QuotaByID(req.Quota); !ok {
				return nil, invalid("quota", "unknown quota %q; valid values: %s", req.Quota, quotaIDs())
			}
		}
		snap, err := s.currentSnapshot()
		if err != nil {
			return nil, err
		}
		out = s.buildQuotas(ctx, snap, req)
		return nil, nil
	})
	return out, err
}

func (s *Service) buildQuotas(ctx context.Context, snap snapshot.Snapshot, req QuotasRequest) *QuotasResult {
	r := snap.Reading
	u := &r.Usage
	res := &QuotasResult{Quotas: []QuotaView{}, Notes: []string{
		"Only RUNNING instances are counted; whether STOPPED A1 instances count against the free limit is not verified here.",
		"GB means 2^30 bytes.",
	}, Meta: s.meta(ctx, &snap, r.Complete())}
	needle := strings.ToLower(strings.TrimSpace(req.NameContains))
	matches := 0

	for _, q := range freetier.Quotas {
		if req.Quota != "" && q.ID != req.Quota {
			continue
		}
		used, limit := q.Measure(u)
		view := QuotaView{
			Quota: q.ID, Name: q.Name, Family: q.Family, Description: q.Description,
			Used: round3(used), Limit: limit, Unit: q.Unit, Percentage: q.Percentage(u),
			WarnThreshold: q.WarnThreshold, Available: r.Available(q.Source), Resources: []Resource{},
		}
		if !view.Available {
			view.ErrorCode = r.Status(q.Source).ErrorCode
		}
		for _, res := range resourcesFor(q.ID, u) {
			if needle == "" || strings.Contains(strings.ToLower(res.Name), needle) {
				view.Resources = append(view.Resources, res)
			}
		}
		if needle != "" {
			if len(view.Resources) == 0 {
				continue
			}
			matches += len(view.Resources)
		}
		res.Quotas = append(res.Quotas, view)
	}
	if needle != "" {
		res.Matches = &matches
		if matches == 0 {
			res.Notes = append(res.Notes, fmt.Sprintf("No resource name contains %q. Names are matched case-insensitively against instances, volumes, buckets and load balancers.", req.NameContains))
		} else if matches > 1 {
			res.Notes = append(res.Notes, fmt.Sprintf("%d resources match %q: they are all listed; do not assume which one was meant.", matches, req.NameContains))
		}
	}
	return res
}

func resourcesFor(id freetier.QuotaID, u *freetier.AllUsage) []Resource {
	var out []Resource
	switch id {
	case freetier.QuotaARMOCPUs, freetier.QuotaARMMemory, freetier.QuotaAMDInstances:
		for _, in := range u.Compute.InstanceDetails {
			isARM := strings.Contains(in.Shape, "A1") || strings.Contains(in.Shape, "Ampere")
			if (id == freetier.QuotaAMDInstances) == isARM {
				continue
			}
			size, unit := 1.0, "count"
			switch id {
			case freetier.QuotaARMOCPUs:
				size, unit = in.OCPUs, "ocpu"
			case freetier.QuotaARMMemory:
				size, unit = in.MemoryGB, "GB"
			}
			out = append(out, Resource{Kind: "instance", Name: in.Name, Size: &size, Unit: unit, SizeKnown: true, State: in.State, Detail: in.Shape})
		}
	case freetier.QuotaBlockStorage:
		for _, v := range u.BlockStorage.Volumes {
			size := float64(v.SizeGB)
			out = append(out, Resource{Kind: v.Kind + "_volume", Name: v.Name, Size: &size, Unit: "GB", SizeKnown: true, State: v.State})
		}
	case freetier.QuotaObjectStorage:
		for _, b := range u.ObjectStorage.Buckets {
			res := Resource{Kind: "bucket", Name: b.Name, Unit: "GB", SizeKnown: b.SizeKnown}
			if b.SizeKnown {
				size := round3(b.SizeGB)
				res.Size = &size
			}
			out = append(out, res)
		}
		sort.SliceStable(out, func(i, j int) bool { return sizeOf(out[i]) > sizeOf(out[j]) })
	case freetier.QuotaLoadBalancers:
		for _, lb := range u.LoadBalancer.LoadBalancers {
			one := 1.0
			out = append(out, Resource{Kind: "load_balancer", Name: lb.Name, Size: &one, Unit: "count", SizeKnown: true, State: lb.State, Detail: lb.Shape})
		}
	}
	return out
}

func sizeOf(r Resource) float64 {
	if r.Size == nil {
		return -1
	}
	return *r.Size
}

func quotaIDs() string {
	ids := make([]string, len(freetier.Quotas))
	for i, q := range freetier.Quotas {
		ids[i] = string(q.ID)
	}
	return strings.Join(ids, ", ")
}

// ---------------------------------------------------------------------------
// assess_billing_risk
// ---------------------------------------------------------------------------

// BillingRisk evalúa si algo se va a facturar este mes.
func (s *Service) BillingRisk(ctx context.Context, _ BillingRequest) (*BillingResult, error) {
	var out *BillingResult
	err := s.call(ctx, OpBilling, audit.ScopeRead, nil, func() ([]string, error) {
		snap, err := s.currentSnapshot()
		if err != nil {
			return nil, err
		}
		out = s.buildBilling(ctx, snap)
		return nil, nil
	})
	return out, err
}

func (s *Service) buildBilling(ctx context.Context, snap snapshot.Snapshot) *BillingResult {
	r := snap.Reading
	u := &r.Usage
	now := s.src.Now()
	proj := freetier.ProjectEgress(u.Bandwidth.EgressGB, u.Bandwidth.LimitGB(), now)

	res := &BillingResult{
		Month:         now.Format("2006-01"),
		DaysElapsed:   round1(proj.DaysElapsed),
		DaysRemaining: round1(float64(proj.DaysInMonth) - proj.DaysElapsed),
		Quotas:        []BillingQuota{},
		Method: fmt.Sprintf("Storage quotas are levels: compared with their warning threshold and limit. Egress is projected linearly from the month-to-date total (used / days elapsed × days in month); no projection before day %.0f.",
			freetier.MinDaysForProjection),
		NotCovered: []string{
			"OCI budget alerts and the actual invoice (only visible in the OCI console)",
			"paid services outside the Always Free list",
			"resources in child compartments (see meta.scope)",
		},
		Meta: s.meta(ctx, &snap, r.Complete()),
	}

	atRisk, unknown := false, false
	for _, q := range freetier.AccruingQuotas() {
		used, limit := q.Measure(u)
		bq := BillingQuota{
			Quota: q.ID, Name: q.Name, Available: r.Available(q.Source), Used: round3(used), Limit: limit,
			Unit: q.Unit, Percentage: q.Percentage(u), WarnThreshold: q.WarnThreshold,
		}
		switch {
		case !bq.Available:
			bq.Status, bq.Reason = "unknown", fmt.Sprintf("could not be read (%s)", r.Status(q.Source).ErrorCode)
			unknown = true
		case used >= limit:
			bq.Status, bq.Reason = "over_limit", "already over the free limit: usage beyond it is billed"
			atRisk = true
		case int(bq.Percentage) >= q.WarnThreshold:
			bq.Status, bq.Reason = "warning", fmt.Sprintf("over its %d%% warning threshold", q.WarnThreshold)
			atRisk = true
		default:
			bq.Status, bq.Reason = "ok", "below its warning threshold"
		}
		if q.ID == freetier.QuotaEgress && bq.Available {
			if !proj.Projectable {
				bq.Reason += "; too early in the month to project"
			} else {
				monthEnd := proj.MonthEndGB
				pct := round1(monthEnd / limit * 100)
				bq.ProjectedMonthEnd, bq.ProjectedPercentage = &monthEnd, &pct
				if proj.CrossesOn != nil {
					day := proj.CrossesOn.Format("2006-01-02")
					bq.CrossesLimitOn = &day
					if bq.Status == "ok" || bq.Status == "warning" {
						bq.Status = "projected_over"
					}
					bq.Reason = fmt.Sprintf("at %.1f GB/day the free %.0f GB run out on %s", proj.RatePerDayGB, limit, day)
					atRisk = true
				} else if bq.Status == "ok" {
					bq.Reason = fmt.Sprintf("at %.1f GB/day the month ends at ~%.0f GB (%.1f%% of the limit)", proj.RatePerDayGB, monthEnd, pct)
				}
			}
		}
		res.Quotas = append(res.Quotas, bq)
	}

	switch {
	case atRisk:
		res.Verdict = VerdictAtRisk
	case unknown:
		res.Verdict = VerdictUnknown
	default:
		res.Verdict = VerdictNoRisk
	}
	return res
}

// ---------------------------------------------------------------------------
// get_saturation_timeline
// ---------------------------------------------------------------------------

var resolutionDurations = map[Resolution]time.Duration{
	Resolution1m: time.Minute,
	Resolution5m: 5 * time.Minute,
	Resolution1h: time.Hour,
}

// Saturation devuelve series de saturación para una ventana.
func (s *Service) Saturation(ctx context.Context, req SaturationRequest) (*SaturationResult, error) {
	var out *SaturationResult
	args := map[string]any{"start": req.Start}
	if req.End != "" {
		args["end"] = req.End
	}
	if req.Resolution != "" {
		args["resolution"] = req.Resolution
	}
	if len(req.Metrics) > 0 {
		args["metrics"] = req.Metrics
	}
	if req.IncludePoints != nil {
		args["includePoints"] = *req.IncludePoints
	}
	err := s.call(ctx, OpSaturation, audit.ScopeRead, args, func() ([]string, error) {
		var err error
		out, err = s.buildSaturation(ctx, req)
		return nil, err
	})
	return out, err
}

func (s *Service) buildSaturation(ctx context.Context, req SaturationRequest) (*SaturationResult, error) {
	now := s.src.Now()
	if !s.src.Configured() {
		return nil, fromSource(source.ErrNotConfigured)
	}

	// --- Validación: cada error dice qué campo y cuál es el valor válido ---
	if req.Start == "" {
		return nil, invalid("start", "start is required (RFC 3339 UTC, e.g. %s)", now.Add(-time.Hour).Format(time.RFC3339))
	}
	start, err := time.Parse(time.RFC3339, req.Start)
	if err != nil {
		return nil, invalid("start", "start %q is not RFC 3339 (e.g. %s)", req.Start, now.Add(-time.Hour).Format(time.RFC3339))
	}
	end := now
	if req.End != "" {
		if end, err = time.Parse(time.RFC3339, req.End); err != nil {
			return nil, invalid("end", "end %q is not RFC 3339", req.End)
		}
	}
	start, end = start.UTC(), end.UTC()
	window := Window{Start: start, End: end}
	if end.After(now) {
		end, window.End = now, now
		window.Note = "end was in the future and has been clamped to now"
	}
	if !end.After(start) {
		return nil, invalid("end", "end (%s) must be after start (%s)", end.Format(time.RFC3339), start.Format(time.RFC3339))
	}
	earliest := now.Add(-RetentionDays * 24 * time.Hour)
	if start.Before(earliest) {
		return nil, &Error{
			Code: CodeOutOfRange, Field: "start",
			Message: fmt.Sprintf("start %s is older than the %d-day OCI Monitoring retention", start.Format(time.RFC3339), RetentionDays),
			Hint:    fmt.Sprintf("the earliest allowed start is %s; data before that no longer exists anywhere this watcher can reach", earliest.Format(time.RFC3339)),
		}
	}

	res := req.Resolution
	if res == "" {
		res = Resolution1m
	}
	step, ok := resolutionDurations[res]
	if !ok {
		return nil, invalid("resolution", "resolution %q is not one of 1m, 5m, 1h", req.Resolution)
	}
	window.Resolution = res
	if span := end.Sub(start); span > MaxWindow[res] {
		return nil, &Error{
			Code: CodeInvalidArgument, Field: "resolution",
			Message: fmt.Sprintf("a %s window is too long for %s resolution (max %s)", span.Round(time.Minute), res, MaxWindow[res]),
			Hint:    "use a coarser resolution (5m allows 7 days, 1h allows 90) or a shorter window",
		}
	}

	metrics := req.Metrics
	if len(metrics) == 0 {
		for _, m := range source.Metrics {
			metrics = append(metrics, m.Metric)
		}
	}
	seen := map[source.Metric]bool{}
	var unique []source.Metric
	for _, m := range metrics {
		if _, ok := source.MetricByName(m); !ok {
			return nil, invalid("metrics", "unknown metric %q; valid values: ingress_bytes, egress_bytes, ingress_throttle_drops, cpu_percent, memory_percent", m)
		}
		if !seen[m] {
			seen[m] = true
			unique = append(unique, m)
		}
	}
	includePoints := req.IncludePoints == nil || *req.IncludePoints
	expected := int(end.Sub(start)/step) * len(unique)
	if includePoints && expected > MaxPointsPerResponse {
		return nil, &Error{
			Code: CodeInvalidArgument, Field: "includePoints",
			Message: fmt.Sprintf("this request would return ~%d points (max %d with includePoints)", expected, MaxPointsPerResponse),
			Hint:    "ask for fewer metrics, a coarser resolution or a shorter window, or set includePoints=false to get only the summary",
		}
	}

	// El límite se aplica después de validar: una petición mal formada no
	// llega a OCI y no tiene por qué gastar cupo.
	client, _ := audit.ClientFrom(ctx)
	if !s.limiter(client.Name).Allow() {
		return nil, &Error{
			Code: CodeRateLimited, Retryable: true, RetryAfterSeconds: 3,
			Message: fmt.Sprintf("timeline queries are limited to %d per minute per client (each one calls OCI Monitoring)", TimelineRequestsPerMinute),
			Hint:    "reuse the series you already have, or ask for several metrics in one call",
		}
	}

	// --- Consulta: una serie por métrica, en paralelo ---
	type result struct {
		series source.Series
		err    error
	}
	results := make([]result, len(unique))
	var wg sync.WaitGroup
	for i, m := range unique {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sr, err := s.src.Series(ctx, source.SeriesQuery{Metric: m, Start: start, End: end, Resolution: step})
			results[i] = result{sr, err}
		}()
	}
	wg.Wait()

	out := &SaturationResult{
		Window:      window,
		Series:      []SeriesView{},
		Summary:     TimelineSummary{Metrics: []MetricSummary{}, ThrottleDropIntervals: []DropInterval{}},
		BucketLabel: "end_of_interval",
		Unavailable: []Unavailable{},
	}
	var newest time.Time
	var firstErr error
	for i, m := range unique {
		spec, _ := source.MetricByName(m)
		if err := results[i].err; err != nil {
			if firstErr == nil {
				firstErr = err
			}
			code, msg := source.Classify(err)
			out.Unavailable = append(out.Unavailable, Unavailable{Signal: string(m), ErrorCode: code, Message: msg, Retryable: source.Retryable(code)})
			continue
		}
		points := results[i].series.Points
		view := SeriesView{
			Metric: m, Unit: spec.Unit, Description: spec.Description, Namespace: spec.Namespace,
			Query: results[i].series.Query, Aggregation: spec.Aggregation, PointCount: len(points), Points: []source.Point{},
		}
		if includePoints {
			view.Points = nonNilPoints(points)
		}
		out.Series = append(out.Series, view)
		out.Summary.Metrics = append(out.Summary.Metrics, summarize(spec, points))
		if len(points) > 0 && points[len(points)-1].T.After(newest) {
			newest = points[len(points)-1].T
		}
		if m == source.MetricIngressDrops {
			total := 0.0
			for _, p := range points {
				total += p.V
			}
			out.Summary.TotalThrottleDrops = &total
			out.Summary.ThrottleDropIntervals = dropIntervals(points, step)
		}
	}
	if len(out.Series) == 0 && firstErr != nil {
		return nil, fromSource(firstErr)
	}

	if !newest.IsZero() {
		out.DataLag = DataLag{NewestPointAt: &newest}
		// El retraso sólo dice algo si la ventana llega hasta ahora; para
		// una ventana de hace tres días sería la antigüedad de la ventana.
		if now.Sub(end) < 15*time.Minute {
			lag := int(now.Sub(newest).Seconds())
			out.DataLag.LagSeconds = &lag
		}
		if end.Sub(newest) > 2*step {
			out.DataLag.Note = fmt.Sprintf("no datapoints after %s: OCI Monitoring publishes a few minutes late, so the end of the window is not visible yet", newest.Format(time.RFC3339))
		} else {
			out.DataLag.Note = "the window is fully published"
		}
	} else {
		out.DataLag.Note = "no datapoints in this window"
	}
	out.Meta = s.meta(ctx, nil, len(out.Unavailable) == 0)
	return out, nil
}

func summarize(spec source.MetricSpec, points []source.Point) MetricSummary {
	ms := MetricSummary{Metric: spec.Metric, Unit: spec.Unit, Points: len(points)}
	if len(points) == 0 {
		return ms
	}
	ms.Min, ms.Max = math.Inf(1), math.Inf(-1)
	sum := 0.0
	for _, p := range points {
		sum += p.V
		if p.V < ms.Min {
			ms.Min = p.V
		}
		if p.V > ms.Max {
			ms.Max = p.V
			t := p.T
			ms.MaxAt = &t
		}
	}
	ms.Mean = round3(sum / float64(len(points)))
	if spec.Aggregation == "sum" {
		total := sum
		ms.Total = &total
	}
	return ms
}

func dropIntervals(points []source.Point, step time.Duration) []DropInterval {
	out := []DropInterval{}
	var cur *DropInterval
	for _, p := range points {
		if p.V <= 0 {
			cur = nil
			continue
		}
		if cur == nil || !p.T.Add(-step).Equal(cur.End) {
			out = append(out, DropInterval{Start: p.T.Add(-step), End: p.T})
			cur = &out[len(out)-1]
		}
		cur.End = p.T
		cur.TotalDrops += p.V
		if p.V > cur.PeakPerBucket {
			cur.PeakPerBucket = p.V
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// get_watcher_diagnostics
// ---------------------------------------------------------------------------

// Diagnostics describe la salud del propio watcher.
func (s *Service) Diagnostics(ctx context.Context, _ DiagnosticsRequest) (*DiagnosticsResult, error) {
	var out *DiagnosticsResult
	err := s.call(ctx, OpDiagnostics, audit.ScopeRead, nil, func() ([]string, error) {
		out = s.buildDiagnostics(ctx)
		return nil, nil
	})
	return out, err
}

func (s *Service) buildDiagnostics(ctx context.Context) *DiagnosticsResult {
	client, _ := audit.ClientFrom(ctx)
	h := s.cfg.Store.Health()
	res := &DiagnosticsResult{
		Health:       HealthHealthy,
		Reasons:      []string{},
		Version:      s.cfg.Version,
		DataSource:   s.src.Kind(),
		Configured:   s.src.Configured(),
		AuthEnforced: s.cfg.Registry == nil || s.cfg.Registry.Enforced(),
		Client:       client.Name,
		ClientScopes: client.ScopeStrings(),
		Snapshot:     SnapshotInfo{RefreshIntervalSeconds: int(s.cfg.RefreshInterval.Seconds())},
		Sources:      []SourceView{},
		Limits: OperationalLimits{
			RefreshCooldownSeconds:    int(s.cfg.Store.Cooldown().Seconds()),
			TimelineRequestsPerMinute: TimelineRequestsPerMinute,
			TimelineWindows: []WindowLimit{
				{Resolution1m, int(MaxWindow[Resolution1m].Hours())},
				{Resolution5m, int(MaxWindow[Resolution5m].Hours())},
				{Resolution1h, int(MaxWindow[Resolution1h].Hours())},
			},
			MaxPointsPerResponse: MaxPointsPerResponse,
			RetentionDays:        RetentionDays,
		},
	}
	if !h.LastAttemptAt.IsZero() {
		t := h.LastAttemptAt.UTC()
		res.Snapshot.LastAttemptAt = &t
	}
	if !h.LastSuccessAt.IsZero() {
		t := h.LastSuccessAt.UTC()
		res.Snapshot.LastSuccessAt = &t
	}
	if h.LastError != nil {
		code, msg := source.Classify(h.LastError)
		res.Snapshot.LastError = &ErrorInfo{Code: code, Message: msg, Retryable: source.Retryable(code)}
	}

	if !res.Configured {
		res.Reasons = append(res.Reasons, "OCI credentials are not configured: nothing can be measured")
	}
	if h.Snapshot == nil {
		if res.Configured {
			res.Reasons = append(res.Reasons, "no snapshot has been read successfully yet")
		}
		res.Meta = s.meta(ctx, nil, false)
	} else {
		snap := *h.Snapshot
		observed := snap.Reading.ObservedAt
		age := int(s.src.Now().Sub(observed).Seconds())
		res.Snapshot.Present, res.Snapshot.ObservedAt, res.Snapshot.AgeSeconds = true, &observed, &age
		res.Snapshot.FetchDurationMs = snap.Duration.Milliseconds()
		for _, name := range freetier.AllSources {
			st := snap.Reading.Status(name)
			res.Sources = append(res.Sources, SourceView{
				Name: name, Available: st.Available, ErrorCode: st.ErrorCode, Message: st.Message, Retryable: source.Retryable(st.ErrorCode),
			})
			if !st.Available {
				res.Reasons = append(res.Reasons, fmt.Sprintf("source %s unavailable (%s)", name, st.ErrorCode))
			}
		}
		if s.stale(snap) {
			res.Reasons = append(res.Reasons, fmt.Sprintf("snapshot is %ds old (refresh interval %s)", age, s.cfg.RefreshInterval))
		}
		res.Meta = s.meta(ctx, &snap, snap.Reading.Complete())
	}
	if res.Snapshot.LastError != nil {
		res.Reasons = append(res.Reasons, fmt.Sprintf("the last read failed (%s)", res.Snapshot.LastError.Code))
	}
	if len(res.Reasons) > 0 {
		res.Health = HealthDegraded
	}
	return res
}

// ---------------------------------------------------------------------------
// refresh_usage_snapshot
// ---------------------------------------------------------------------------

// Refresh pide una lectura nueva a OCI.
func (s *Service) Refresh(ctx context.Context, req RefreshRequest) (*RefreshResult, error) {
	var out *RefreshResult
	args := map[string]any{}
	if req.Reason != "" {
		args["reason"] = truncate(req.Reason, 200)
	}
	err := s.call(ctx, OpRefresh, audit.ScopeRefresh, args, func() ([]string, error) {
		r := s.cfg.Store.Refresh(ctx, false)
		if r.Err != nil && r.Snapshot == nil {
			return nil, fromSource(r.Err)
		}
		out = &RefreshResult{Refreshed: r.Refreshed, Reason: r.Reason, Unavailable: []Unavailable{}}
		if r.RetryAfter > 0 {
			out.RetryAfterSeconds = int(math.Ceil(r.RetryAfter.Seconds()))
		}
		if r.Snapshot != nil {
			out.DurationMs = r.Snapshot.Duration.Milliseconds()
			out.Complete = r.Snapshot.Reading.Complete()
			out.Unavailable = unavailableOf(r.Snapshot.Reading)
			out.Meta = s.meta(ctx, r.Snapshot, out.Complete)
		} else {
			out.Meta = s.meta(ctx, nil, false)
		}
		if r.Reason == snapshot.ReasonRefreshed && r.Refreshed {
			return []string{"snapshot"}, nil
		}
		return nil, nil
	})
	return out, err
}

// ---------------------------------------------------------------------------
// get_audit_log (sólo REST: es para el humano que pregunta qué hizo el agente)
// ---------------------------------------------------------------------------

// AuditLog devuelve eventos del registro de auditoría.
func (s *Service) AuditLog(ctx context.Context, req AuditRequest) (*AuditResult, error) {
	var out *AuditResult
	err := s.call(ctx, OpAudit, audit.ScopeAudit, nil, func() ([]string, error) {
		if req.Limit < 0 || req.Limit > 500 {
			return nil, invalid("limit", "limit must be between 1 and 500")
		}
		if req.Outcome != "" && req.Outcome != audit.OutcomeOK && req.Outcome != audit.OutcomeError && req.Outcome != audit.OutcomeDenied {
			return nil, invalid("outcome", "outcome must be ok, error or denied")
		}
		out = &AuditResult{
			Events: s.cfg.Audit.Query(audit.Filter{Client: req.Client, Operation: req.Operation, Outcome: req.Outcome, Limit: req.Limit}),
			Meta:   s.meta(ctx, nil, true),
		}
		return nil, nil
	})
	return out, err
}

// ---------------------------------------------------------------------------

func unavailableOf(r freetier.Reading) []Unavailable {
	out := []Unavailable{}
	for _, name := range freetier.AllSources {
		if st := r.Status(name); !st.Available {
			out = append(out, Unavailable{Signal: name, ErrorCode: st.ErrorCode, Message: st.Message, Retryable: source.Retryable(st.ErrorCode)})
		}
	}
	return out
}

func anyRetryable(us []Unavailable) bool {
	for _, u := range us {
		if u.Retryable {
			return true
		}
	}
	return false
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func nonNilPoints(p []source.Point) []source.Point {
	if p == nil {
		return []source.Point{}
	}
	return p
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }
func round3(v float64) float64 { return math.Round(v*1000) / 1000 }

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
