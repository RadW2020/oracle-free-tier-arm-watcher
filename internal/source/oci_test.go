package source

import (
	"context"
	"testing"
)

// TestOCIConfigured verifica la validación de credenciales
func TestOCIConfigured(t *testing.T) {
	full := OCIConfig{
		TenancyID:      "ocid1.tenancy.test",
		UserID:         "ocid1.user.test",
		Fingerprint:    "aa:bb:cc:dd",
		PrivateKeyPath: "/tmp/test.pem",
		Region:         "us-ashburn-1",
	}
	tests := []struct {
		name     string
		cfg      OCIConfig
		expected bool
	}{
		{"todas las variables configuradas", full, true},
		{"falta OCI_TENANCY_ID", func() OCIConfig { c := full; c.TenancyID = ""; return c }(), false},
		{"ninguna variable configurada", OCIConfig{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewOCI(tt.cfg).Configured(); got != tt.expected {
				t.Errorf("Configured() = %v; want %v", got, tt.expected)
			}
		})
	}
}

func TestOCIScopeSaysChildCompartmentsAreExcluded(t *testing.T) {
	scope := NewOCI(OCIConfig{TenancyID: "ocid1.tenancy.x"}).Scope()
	if scope != "compartment ocid1.tenancy.x only (child compartments are not included)" {
		t.Errorf("scope = %q", scope)
	}
}

func TestOCIUnreadableKeyIsClassified(t *testing.T) {
	o := NewOCI(OCIConfig{TenancyID: "t", UserID: "u", Fingerprint: "f", PrivateKeyPath: "/nonexistent.pem", Region: "r"})
	_, err := o.Read(context.Background())
	if code, _ := Classify(err); code != "credentials_unreadable" {
		t.Errorf("code = %q; want credentials_unreadable", code)
	}
}

// BenchmarkOCIRead mide una lectura real (requiere credenciales reales).
// Se salta si no hay credenciales configuradas.
func BenchmarkOCIRead(b *testing.B) {
	o := NewOCI(OCIConfigFromEnv())
	if !o.Configured() {
		b.Skip("OCI not configured, skipping benchmark")
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := o.Read(context.Background()); err != nil {
			b.Fatalf("Read() error = %v", err)
		}
	}
}
