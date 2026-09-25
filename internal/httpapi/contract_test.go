package httpapi

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/RadW2020/oracle-free-tier-arm-watcher/internal/freetier"
)

// Estos tests fijan el contrato JSON de los endpoints heredados ANTES de
// refactorizar nada. Checkly asierta $.maxUsagePercentage, $.configured y
// $.usage.bandwidth.percentage, y el 18/09/2026 un omitempty bastó para
// romperlo. Cualquier cambio aquí tiene que ser aditivo: todas las rutas del
// golden deben seguir existiendo con el mismo valor.

var updateGolden = flag.Bool("update", false, "regenerate golden files")

// contractUsage es el uso con el que se generó el golden antes del refactor:
// la cuenta real, con la asignación al 100 % y la cuota acumulativa casi
// vacía.
func contractUsage() freetier.Reading {
	var usage freetier.AllUsage
	usage.Compute.ARM.OCPUs = freetier.UsageMetric{Used: 4, Limit: 4, Percentage: 100}
	usage.Compute.ARM.MemoryGB = freetier.UsageMetric{Used: 24, Limit: 24, Percentage: 100}
	usage.Compute.ARM.Instances = 1
	usage.Compute.AMD.Instances.Percentage = 100
	usage.Compute.TotalInstances = 1
	usage.BlockStorage.BootVolumes.Count = 1
	usage.BlockStorage.BootVolumes.SizeGB = 200
	usage.BlockStorage.Total = freetier.UsageMetric{Used: 200, Limit: 200, Percentage: 100}
	usage.PublicIPs.Percentage = 50
	usage.Database.AutonomousDBs.Percentage = 100
	usage.ObjectStorage.Buckets = []freetier.BucketInfo{{Name: "postiz-media", SizeGB: 0.1, SizeKnown: true}}
	usage.ObjectStorage.Total = freetier.UsageMetric{Used: 0.1, Limit: 10, Percentage: 1}
	usage.LoadBalancer.Count = freetier.UsageMetric{Used: 0, Limit: 1, Percentage: 0}
	usage.LoadBalancer.LoadBalancers = []freetier.LoadBalancerInfo{}
	usage.Bandwidth = freetier.BandwidthUsage{EgressGB: 144.5, LimitTB: 10, Percentage: 1}
	usage.Saturation = freetier.SaturationUsage{CPUPercentage: 6.2, IngressMBPerMin: 7.9, EgressMBPerMin: 5.9, SampleAgeSeconds: 180}
	return freetier.NewReading(usage, time.Date(2026, 9, 17, 18, 30, 0, 0, time.UTC), nil)
}

func TestLegacyUsageContract(t *testing.T) {
	reading := contractUsage()
	assessment := freetier.Assess(reading)
	body := mustJSON(t, UsageResponse{
		Status:               assessment.Status,
		MaxUsagePercentage:   assessment.AccruingPercentage,
		AllocationPercentage: assessment.AllocationPercentage,
		Warnings:             legacyWarnings(reading, assessment),
		Timestamp:            "2026-09-17T18:30:00Z",
		Configured:           true,
		Usage:                &reading.Usage,
		FreeTierLimits:       freetier.Limits,
		Complete:             reading.Complete(),
		Sources:              reading.Sources,
	})
	assertGoldenSubset(t, "legacy_usage.golden.json", body)
	assertPaths(t, body, "maxUsagePercentage", "configured", "usage.bandwidth.percentage")
}

func TestLegacyStatusContract(t *testing.T) {
	reading := contractUsage()
	assessment := freetier.Assess(reading)
	complete := reading.Complete()
	body := mustJSON(t, StatusResponse{
		Status:               assessment.Status,
		MaxUsagePercentage:   assessment.AccruingPercentage,
		AllocationPercentage: assessment.AllocationPercentage,
		Warnings:             legacyWarnings(reading, assessment),
		Timestamp:            "2026-09-17T18:30:00Z",
		Complete:             &complete,
	})
	assertGoldenSubset(t, "legacy_status.golden.json", body)
	assertPaths(t, body, "maxUsagePercentage", "allocationPercentage", "status")
}

// TestStatusResponseAlwaysCarriesTheNumber: el check de Checkly asierta
// $.maxUsagePercentage. Con `omitempty` el campo desaparecia al valer 0, que
// es el valor normal desde que el estado solo mide cuota acumulativa.
func TestStatusResponseAlwaysCarriesTheNumber(t *testing.T) {
	body := mustJSON(t, StatusResponse{Status: "OK", MaxUsagePercentage: 0, AllocationPercentage: 100})
	if _, ok := body["maxUsagePercentage"]; !ok {
		t.Errorf("falta maxUsagePercentage en %v", body)
	}
	if _, ok := body["allocationPercentage"]; !ok {
		t.Errorf("falta allocationPercentage en %v", body)
	}
}

func mustJSON(t *testing.T, v any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return decoded
}

// assertGoldenSubset exige que todo lo que hay en el golden siga igual en la
// respuesta. Campos nuevos en la respuesta están permitidos: así se distingue
// un cambio aditivo de uno que rompe a Checkly.
func assertGoldenSubset(t *testing.T, name string, got map[string]any) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *updateGolden {
		raw, _ := json.MarshalIndent(got, "", "  ")
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, append(raw, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("golden %s: %v (run with -update to create it)", path, err)
	}
	var want map[string]any
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatalf("golden %s: %v", path, err)
	}
	for _, diff := range subsetDiff("$", want, got) {
		t.Error(diff)
	}
}

func subsetDiff(path string, want, got any) []string {
	switch w := want.(type) {
	case map[string]any:
		g, ok := got.(map[string]any)
		if !ok {
			return []string{path + ": expected an object"}
		}
		var diffs []string
		for k, wv := range w {
			gv, ok := g[k]
			if !ok {
				diffs = append(diffs, path+"."+k+": missing")
				continue
			}
			diffs = append(diffs, subsetDiff(path+"."+k, wv, gv)...)
		}
		return diffs
	case []any:
		g, ok := got.([]any)
		if !ok || len(g) < len(w) {
			return []string{path + ": expected an array with at least the golden elements"}
		}
		var diffs []string
		for i := range w {
			diffs = append(diffs, subsetDiff(path+"[]", w[i], g[i])...)
		}
		return diffs
	default:
		if want != got {
			raw, _ := json.Marshal(got)
			wraw, _ := json.Marshal(want)
			return []string{path + ": got " + string(raw) + ", want " + string(wraw)}
		}
		return nil
	}
}

func assertPaths(t *testing.T, body map[string]any, paths ...string) {
	t.Helper()
	for _, p := range paths {
		var cur any = body
		for _, part := range splitPath(p) {
			m, ok := cur.(map[string]any)
			if !ok {
				t.Errorf("%s: not an object at %q", p, part)
				break
			}
			if cur, ok = m[part]; !ok {
				t.Errorf("%s: missing (Checkly asserts on it)", p)
				break
			}
		}
	}
}

func splitPath(p string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(p); i++ {
		if p[i] == '.' {
			parts = append(parts, p[start:i])
			start = i + 1
		}
	}
	return append(parts, p[start:])
}
