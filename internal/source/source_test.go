package source

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/RadW2020/oracle-free-tier-arm-watcher/internal/freetier"
)

func mustFixture(t *testing.T, name string) *Fixture {
	t.Helper()
	sc, err := LoadScenario(name)
	if err != nil {
		t.Fatal(err)
	}
	return NewFixture(sc)
}

func TestAllScenariosLoad(t *testing.T) {
	names := ScenarioNames()
	if len(names) < 5 {
		t.Fatalf("scenarios = %v; want at least 5", names)
	}
	for _, name := range names {
		sc, err := LoadScenario(name)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if !sc.Synthetic {
			t.Errorf("%s: every bundled scenario must be marked synthetic", name)
		}
	}
	if _, err := LoadScenario("nope"); err == nil {
		t.Error("an unknown scenario must fail with the list of valid ones")
	}
}

// TestIncidentMatchesPostmortem: el escenario sólo sirve si reproduce las
// cifras del postmortem, que es lo que las evals van a preguntar.
func TestIncidentMatchesPostmortem(t *testing.T) {
	f := mustFixture(t, "incident-2026-09-17")
	ctx := context.Background()

	drops, err := f.Series(ctx, SeriesQuery{
		Metric: MetricIngressDrops, Resolution: time.Minute,
		Start: time.Date(2026, 9, 17, 15, 0, 0, 0, time.UTC),
		End:   time.Date(2026, 9, 17, 15, 30, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	var total float64
	var first, last time.Time
	for _, p := range drops.Points {
		if p.V > 0 {
			if first.IsZero() {
				first = p.T
			}
			last = p.T
			total += p.V
		}
	}
	if total != 81000 {
		t.Errorf("drops = %v; want 81000", total)
	}
	if !first.Equal(time.Date(2026, 9, 17, 15, 15, 0, 0, time.UTC)) || !last.Equal(time.Date(2026, 9, 17, 15, 20, 0, 0, time.UTC)) {
		t.Errorf("drops between %v and %v; want 15:15-15:20 (bucket end labels)", first, last)
	}
	// El minuto del fallo de Checkly (15:13:12) no tiene descartes: el
	// postmortem lo deja sin atribuir y el escenario también.
	for _, p := range drops.Points {
		if p.T.Equal(time.Date(2026, 9, 17, 15, 14, 0, 0, time.UTC)) && p.V != 0 {
			t.Errorf("15:13-15:14 has %v drops; want 0", p.V)
		}
	}
	if drops.Query != "VnicIngressDropsThrottle[1m].sum()" {
		t.Errorf("query = %q", drops.Query)
	}

	// La descarga del día 16 es igual: así se puede comparar entre días.
	prev, _ := f.Series(ctx, SeriesQuery{
		Metric: MetricIngressDrops, Resolution: time.Hour,
		Start: time.Date(2026, 9, 16, 15, 0, 0, 0, time.UTC),
		End:   time.Date(2026, 9, 16, 16, 0, 0, 0, time.UTC),
	})
	if len(prev.Points) != 1 || prev.Points[0].V != 81000 {
		t.Errorf("hourly drops on the 16th = %+v; want one bucket of 81000", prev.Points)
	}
}

func TestFixtureAggregation(t *testing.T) {
	f := mustFixture(t, "incident-2026-09-17")
	ctx := context.Background()
	window := SeriesQuery{Start: time.Date(2026, 9, 17, 15, 0, 0, 0, time.UTC), End: time.Date(2026, 9, 17, 17, 0, 0, 0, time.UTC)}

	window.Metric, window.Resolution = MetricCPU, 5*time.Minute
	cpu, _ := f.Series(ctx, window)
	if len(cpu.Points) != 24 {
		t.Fatalf("5m cpu points = %d; want 24", len(cpu.Points))
	}
	for _, p := range cpu.Points {
		if p.V > 36 {
			t.Errorf("a mean can't exceed the plateau: %v at %v", p.V, p.T)
		}
		if p.T.Minute()%5 != 0 {
			t.Errorf("bucket %v is not labelled at the end of a 5m interval", p.T)
		}
	}

	window.Metric, window.Resolution = MetricIngressDrops, 5*time.Minute
	drops, _ := f.Series(ctx, window)
	var total float64
	for _, p := range drops.Points {
		total += p.V
	}
	if total != 81000 {
		t.Errorf("summed drops at 5m = %v; want 81000 (sums must survive aggregation)", total)
	}
}

func TestFixtureRespectsPublicationLag(t *testing.T) {
	f := mustFixture(t, "quiet")
	s, _ := f.Series(context.Background(), SeriesQuery{
		Metric: MetricCPU, Resolution: time.Minute, Start: f.Now().Add(-30 * time.Minute), End: f.Now(),
	})
	newest := s.Points[len(s.Points)-1].T
	if lag := f.Now().Sub(newest); lag < 3*time.Minute {
		t.Errorf("newest point is %v old; Monitoring publishes with a lag", lag)
	}
}

func TestFixtureReadDerivesSaturationFromSeries(t *testing.T) {
	r, err := mustFixture(t, "incident-2026-09-17").Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// A las 18:30 la última hora (17:30-18:27 por el retraso de publicación)
	// pilla el final de la re-descarga de las 17:27-17:32: dos minutos de
	// 9.000 descartes, supuesto del escenario.
	if got := r.Usage.Saturation.IngressDropsLastHour; got != 18000 {
		t.Errorf("drops last hour = %v; want 18000", got)
	}
	if r.Usage.Saturation.SampleAgeSeconds < 180 {
		t.Errorf("sample age = %d; want at least the 180s lag", r.Usage.Saturation.SampleAgeSeconds)
	}
	if !r.Complete() {
		t.Errorf("incident reading should be complete: %+v", r.Sources)
	}
	if r.Usage.Compute.ARM.OCPUs.Percentage != 100 {
		t.Errorf("percentages must be computed by Finalize: %+v", r.Usage.Compute.ARM.OCPUs)
	}
}

func TestFixtureFailuresAreUnknownNotZero(t *testing.T) {
	r, err := mustFixture(t, "egress-unavailable").Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if r.Available(freetier.SourceBandwidth) {
		t.Fatal("bandwidth must be unavailable")
	}
	if s := r.Status(freetier.SourceBandwidth); s.ErrorCode != "oci_throttled" {
		t.Errorf("status = %+v", s)
	}
	if r.Usage.Bandwidth.Error == "" {
		t.Error("the legacy section must carry the error, as OCI does")
	}
	if v := freetier.Assess(r).Verdict; v != freetier.StatusUnknown {
		t.Errorf("verdict = %q; want UNKNOWN", v)
	}
}

func TestNotConfigured(t *testing.T) {
	f := mustFixture(t, "not-configured")
	if f.Configured() {
		t.Fatal("scenario says configured=false")
	}
	if _, err := f.Read(context.Background()); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("err = %v; want ErrNotConfigured", err)
	}
	if code, _ := Classify(ErrNotConfigured); code != "not_configured" {
		t.Errorf("code = %q", code)
	}
}

type fakeServiceError struct {
	status int
	code   string
}

func (e fakeServiceError) Error() string           { return fmt.Sprintf("%d %s ocid1.secret", e.status, e.code) }
func (e fakeServiceError) GetHTTPStatusCode() int  { return e.status }
func (e fakeServiceError) GetMessage() string      { return "message with ocid1.tenancy.secret" }
func (e fakeServiceError) GetCode() string         { return e.code }
func (e fakeServiceError) GetOpcRequestID() string { return "req-123" }

func TestClassify(t *testing.T) {
	tests := []struct {
		err       error
		code      string
		retryable bool
	}{
		{fakeServiceError{http.StatusUnauthorized, "NotAuthenticated"}, "oci_auth", false},
		{fakeServiceError{http.StatusNotFound, "NotAuthorizedOrNotFound"}, "oci_permission", false},
		{fakeServiceError{http.StatusTooManyRequests, "TooManyRequests"}, "oci_throttled", true},
		{fakeServiceError{http.StatusBadGateway, "BadGateway"}, "oci_unavailable", true},
		{fmt.Errorf("wrapped: %w", context.DeadlineExceeded), "timeout", true},
		{errors.New("boom"), "internal", false},
	}
	for _, tt := range tests {
		code, message := Classify(tt.err)
		if code != tt.code {
			t.Errorf("Classify(%v) = %q; want %q", tt.err, code, tt.code)
		}
		if Retryable(code) != tt.retryable {
			t.Errorf("Retryable(%q) = %v; want %v", code, Retryable(code), tt.retryable)
		}
		if containsOCID(message) {
			t.Errorf("client message leaks an OCID: %q", message)
		}
	}
}

func containsOCID(s string) bool {
	for i := 0; i+5 <= len(s); i++ {
		if s[i:i+5] == "ocid1" {
			return true
		}
	}
	return false
}

func TestMetricQueries(t *testing.T) {
	spec, _ := MetricByName(MetricIngressBytes)
	if q := spec.Query(time.Hour); q != "VnicFromNetworkBytes[1h].sum()" {
		t.Errorf("query = %q", q)
	}
	spec, _ = MetricByName(MetricCPU)
	if q := spec.Query(5 * time.Minute); q != "CpuUtilization[5m].mean()" {
		t.Errorf("query = %q", q)
	}
}
