package main

import (
	"strings"
	"testing"
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

// TestAllocatedQuotaDoesNotEscalate es la razon de ser de assessQuotas: la
// maquina llevaba meses en CRITICAL por estar usando entero lo que es gratis.
func TestAllocatedQuotaDoesNotEscalate(t *testing.T) {
	assessment := assessQuotas(fullyAllocated())

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

			assessment := assessQuotas(usage)

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

	assessment := assessQuotas(usage)

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
	if w := assessQuotas(usage).Warnings; len(w) != 0 {
		t.Errorf("a 79%% warnings = %v; want ninguno", w)
	}

	usage.ObjectStorage.Total.Percentage = 80
	w := assessQuotas(usage).Warnings
	if len(w) != 1 || !strings.Contains(w[0], "Object Storage at 80%") {
		t.Errorf("a 80%% warnings = %v; want uno sobre Object Storage", w)
	}
}
