package freetier

import (
	"strings"
	"testing"
	"time"
)

// fullyAllocated devuelve el uso real de esta cuenta: las 4 OCPUs ARM, los
// 24 GB de RAM y los 200 GB de disco asignados al completo, que es como se
// aprovecha un Free Tier, y las cuotas acumulativas practicamente vacias.
func fullyAllocated() *AllUsage {
	usage := &AllUsage{}
	usage.Compute.ARM.OCPUs.Percentage = 100
	usage.Compute.ARM.MemoryGB.Percentage = 100
	usage.Compute.AMD.Instances.Percentage = 100
	usage.BlockStorage.Total.Percentage = 100
	usage.PublicIPs.Percentage = 50
	usage.Database.AutonomousDBs.Percentage = 100
	usage.ObjectStorage.Total.Percentage = 1
	usage.Database.StorageUsage.Percentage = 0
	usage.Bandwidth.Percentage = 0
	return usage
}

// available envuelve un uso en una lectura con todas las fuentes al día.
func available(u *AllUsage) Reading {
	return NewReading(*u, time.Date(2026, 9, 17, 18, 30, 0, 0, time.UTC), nil)
}

// TestAllocatedQuotaDoesNotEscalate es la razon de ser de Assess: la
// maquina llevaba meses en CRITICAL por estar usando entero lo que es gratis.
func TestAllocatedQuotaDoesNotEscalate(t *testing.T) {
	assessment := Assess(available(fullyAllocated()))

	if assessment.Status != "OK" {
		t.Errorf("status = %q; want OK (todo lo lleno esta asignado por diseño)", assessment.Status)
	}
	if assessment.AllocationPercentage != 100 {
		t.Errorf("AllocationPercentage = %d; want 100 (debe seguir siendo visible)", assessment.AllocationPercentage)
	}
	if assessment.AccruingPercentage != 1 {
		t.Errorf("AccruingPercentage = %d; want 1", assessment.AccruingPercentage)
	}
	if len(assessment.Warnings) != 0 {
		t.Errorf("warnings = %v; want ninguno", assessment.Warnings)
	}
}

func TestAccruingQuotaEscalates(t *testing.T) {
	tests := []struct {
		name           string
		objectStorage  int
		dbStorage      int
		bandwidth      int
		expectedStatus string
	}{
		{"vacio", 0, 0, 0, "OK"},
		{"object storage a la mitad", 50, 0, 0, "OK"},
		{"object storage al 60", 60, 0, 0, "ATTENTION"},
		{"db storage al 80", 0, 80, 0, "WARNING"},
		{"object storage al 95", 95, 0, 0, "CRITICAL"},
		{"bandwidth al 92", 0, 0, 92, "CRITICAL"},
		{"manda el mayor", 61, 85, 20, "WARNING"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			usage := fullyAllocated()
			usage.ObjectStorage.Total.Percentage = tt.objectStorage
			usage.Database.StorageUsage.Percentage = tt.dbStorage
			usage.Bandwidth.Percentage = tt.bandwidth

			assessment := Assess(available(usage))

			if assessment.Status != tt.expectedStatus {
				t.Errorf("status = %q; want %q", assessment.Status, tt.expectedStatus)
			}
		})
	}
}

// TestBandwidthWarnsEarlier: 10 TB se van muy deprisa si algo se desmadra, y
// a 80 % ya no da tiempo a reaccionar. Por eso avisa a la mitad.
func TestBandwidthWarnsEarlier(t *testing.T) {
	usage := fullyAllocated()
	usage.Bandwidth.Percentage = 55
	usage.Bandwidth.EgressGB = 5600.0
	usage.Bandwidth.LimitTB = 10

	assessment := Assess(available(usage))

	if len(assessment.Warnings) != 1 {
		t.Fatalf("warnings = %v; want 1", assessment.Warnings)
	}
	if !strings.Contains(assessment.Warnings[0], "Bandwidth at 55%") {
		t.Errorf("warning = %q; want que mencione el porcentaje", assessment.Warnings[0])
	}
	if !strings.Contains(assessment.Warnings[0], "5600.0 GB / 10 TB") {
		t.Errorf("warning = %q; want que traiga las cifras absolutas", assessment.Warnings[0])
	}
	// 55 % no llega a 60: avisa sin escalar el estado.
	if assessment.Status != "OK" {
		t.Errorf("status = %q; want OK", assessment.Status)
	}
}

// TestObjectStorageWarnsAtEighty comprueba el umbral del resto de cuotas
// acumulativas, que no comparten el adelanto del bandwidth.
func TestObjectStorageWarnsAtEighty(t *testing.T) {
	usage := fullyAllocated()
	usage.ObjectStorage.Total.Percentage = 79
	if w := Assess(available(usage)).Warnings; len(w) != 0 {
		t.Errorf("a 79%% warnings = %v; want ninguno", w)
	}

	usage.ObjectStorage.Total.Percentage = 80
	w := Assess(available(usage)).Warnings
	if len(w) != 1 || !strings.Contains(w[0], "Object Storage at 80%") {
		t.Errorf("a 80%% warnings = %v; want uno sobre Object Storage", w)
	}
}

// TestUnknownIsNotOK: una consulta de bandwidth caída dejaba el porcentaje a
// 0 y el estado en OK. El estado heredado se queda como estaba (contrato),
// pero el veredicto tiene que decir que no se sabe.
func TestUnknownIsNotOK(t *testing.T) {
	r := NewReading(*fullyAllocated(), time.Now(), map[string]SourceStatus{
		SourceBandwidth: {ErrorCode: "oci_throttled"},
	})
	a := Assess(r)

	if a.Status != StatusOK {
		t.Errorf("legacy status = %q; want OK (no cambia el contrato)", a.Status)
	}
	if a.Verdict != StatusUnknown {
		t.Errorf("verdict = %q; want UNKNOWN", a.Verdict)
	}
	if a.Complete() || len(a.Unavailable) != 1 || a.Unavailable[0] != "Bandwidth" {
		t.Errorf("unavailable = %v; want [Bandwidth]", a.Unavailable)
	}
}

// TestKnownCriticalBeatsUnknown: que falte un dato no puede esconder uno
// malo que sí se conoce.
func TestKnownCriticalBeatsUnknown(t *testing.T) {
	u := fullyAllocated()
	u.ObjectStorage.Total.Percentage = 95
	r := NewReading(*u, time.Now(), map[string]SourceStatus{
		SourceBandwidth: {ErrorCode: "timeout"},
	})

	if v := Assess(r).Verdict; v != StatusCritical {
		t.Errorf("verdict = %q; want CRITICAL", v)
	}
}

func TestUnreportedSourceIsUnavailable(t *testing.T) {
	r := Reading{Usage: *fullyAllocated()}
	if r.Available(SourceBandwidth) {
		t.Error("una fuente que no aparece no puede darse por buena")
	}
	if Assess(r).Verdict != StatusUnknown {
		t.Error("sin fuentes el veredicto tiene que ser UNKNOWN")
	}
}

func TestFinalizeComputesPercentagesInOnePlace(t *testing.T) {
	var u AllUsage
	u.Compute.ARM.OCPUs.Used = 4
	u.ObjectStorage.Total.Used = 0.09
	u.Bandwidth.EgressGB = 5120
	Finalize(&u)

	if u.Compute.ARM.OCPUs.Percentage != 100 || u.Compute.ARM.OCPUs.Limit != 4 {
		t.Errorf("ocpus = %+v", u.Compute.ARM.OCPUs)
	}
	if u.Bandwidth.Percentage != 50 || u.Bandwidth.LimitTB != 10 {
		t.Errorf("bandwidth = %+v", u.Bandwidth)
	}
	// La API heredada trunca; la /v1 no se come el 0,9 %.
	q, _ := QuotaByID(QuotaObjectStorage)
	if u.ObjectStorage.Total.Percentage != 0 || q.Percentage(&u) != 0.9 {
		t.Errorf("object storage legacy=%d v1=%v; want 0 and 0.9", u.ObjectStorage.Total.Percentage, q.Percentage(&u))
	}
	if u.ObjectStorage.Buckets == nil || u.LoadBalancer.LoadBalancers == nil {
		t.Error("las listas vacías tienen que serializarse como [], no como null")
	}
}

func TestProjectEgress(t *testing.T) {
	limit := 10240.0

	t.Run("mes tranquilo", func(t *testing.T) {
		// 8,5 GB/día, el tráfico real de la máquina.
		now := time.Date(2026, 9, 25, 0, 0, 0, 0, time.UTC)
		p := ProjectEgress(8.5*24, limit, now)
		if !p.Projectable || p.CrossesOn != nil {
			t.Fatalf("projection = %+v", p)
		}
		if p.MonthEndGB != 255 {
			t.Errorf("month end = %v; want 255", p.MonthEndGB)
		}
	})

	t.Run("cruza el límite este mes", func(t *testing.T) {
		now := time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)
		p := ProjectEgress(4700, limit, now)
		if p.CrossesOn == nil {
			t.Fatalf("projection = %+v; want a crossing date", p)
		}
		// 4.700 GB en 11,5 días = 408,7 GB/día: los 10.240 GB caen el 26 a la 01:19.
		if want := time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC); !p.CrossesOn.Equal(want) {
			t.Errorf("crosses on %v; want %v", p.CrossesOn, want)
		}
	})

	t.Run("principio de mes no se proyecta", func(t *testing.T) {
		p := ProjectEgress(300, limit, time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC))
		if p.Projectable || p.NotProjection != "insufficient_history" {
			t.Errorf("projection = %+v; want insufficient_history", p)
		}
	})
}

// TestNewMetricPercentage verifica el cálculo de porcentajes
func TestNewMetricPercentage(t *testing.T) {
	tests := []struct {
		name     string
		used     float64
		limit    float64
		expected int
	}{
		{"50% de uso", 2.0, 4.0, 50},
		{"75% de uso", 3.0, 4.0, 75},
		{"100% de uso", 4.0, 4.0, 100},
		{"0% de uso", 0.0, 4.0, 0},
		{"límite cero no divide por cero", 1.0, 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewMetric(tt.used, tt.limit).Percentage; got != tt.expected {
				t.Errorf("Percentage calculation = %d; want %d", got, tt.expected)
			}
		})
	}
}
