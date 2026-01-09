package main

import (
	"testing"
)

// TestGetEnv verifica la función helper getEnv
func TestGetEnv(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		defaultValue string
		envValue     string
		expected     string
	}{
		{
			name:         "usar valor por defecto si variable no existe",
			key:          "NON_EXISTENT_VAR",
			defaultValue: "default",
			envValue:     "",
			expected:     "default",
		},
		{
			name:         "usar valor de entorno si existe",
			key:          "TEST_VAR",
			defaultValue: "default",
			envValue:     "custom",
			expected:     "custom",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Configurar entorno para el test
			if tt.envValue != "" {
				t.Setenv(tt.key, tt.envValue)
			}

			// Ejecutar función
			result := getEnv(tt.key, tt.defaultValue)

			// Verificar resultado
			if result != tt.expected {
				t.Errorf("getEnv(%q, %q) = %q; want %q",
					tt.key, tt.defaultValue, result, tt.expected)
			}
		})
	}
}

// TestUsageMetricPercentage verifica el cálculo de porcentajes
func TestUsageMetricPercentage(t *testing.T) {
	tests := []struct {
		name     string
		used     float64
		limit    float64
		expected int
	}{
		{
			name:     "50% de uso",
			used:     2.0,
			limit:    4.0,
			expected: 50,
		},
		{
			name:     "75% de uso",
			used:     3.0,
			limit:    4.0,
			expected: 75,
		},
		{
			name:     "100% de uso",
			used:     4.0,
			limit:    4.0,
			expected: 100,
		},
		{
			name:     "0% de uso",
			used:     0.0,
			limit:    4.0,
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metric := UsageMetric{
				Used:  tt.used,
				Limit: tt.limit,
			}

			// Calcular porcentaje como lo hace el código
			percentage := int((metric.Used / metric.Limit) * 100)

			if percentage != tt.expected {
				t.Errorf("Percentage calculation = %d; want %d", percentage, tt.expected)
			}
		})
	}
}

// TestIsConfigured verifica la validación de credenciales
func TestIsConfigured(t *testing.T) {
	tests := []struct {
		name     string
		envVars  map[string]string
		expected bool
	}{
		{
			name: "todas las variables configuradas",
			envVars: map[string]string{
				"OCI_TENANCY_ID":       "ocid1.tenancy.test",
				"OCI_USER_ID":          "ocid1.user.test",
				"OCI_FINGERPRINT":      "aa:bb:cc:dd",
				"OCI_PRIVATE_KEY_PATH": "/tmp/test.pem",
				"OCI_REGION":           "us-ashburn-1",
			},
			expected: true,
		},
		{
			name: "falta OCI_TENANCY_ID",
			envVars: map[string]string{
				"OCI_USER_ID":          "ocid1.user.test",
				"OCI_FINGERPRINT":      "aa:bb:cc:dd",
				"OCI_PRIVATE_KEY_PATH": "/tmp/test.pem",
				"OCI_REGION":           "us-ashburn-1",
			},
			expected: false,
		},
		{
			name:     "ninguna variable configurada",
			envVars:  map[string]string{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Limpiar variables de entorno
			for _, key := range []string{
				"OCI_TENANCY_ID",
				"OCI_USER_ID",
				"OCI_FINGERPRINT",
				"OCI_PRIVATE_KEY_PATH",
				"OCI_REGION",
			} {
				t.Setenv(key, "")
			}

			// Configurar variables del test
			for key, value := range tt.envVars {
				t.Setenv(key, value)
			}

			// Ejecutar función
			result := isConfigured()

			// Verificar resultado
			if result != tt.expected {
				t.Errorf("isConfigured() = %v; want %v", result, tt.expected)
			}
		})
	}
}

// BenchmarkGetOCIUsage benchmarks the OCI usage fetching (requiere credenciales reales)
// Este test se salta si no hay credenciales configuradas
func BenchmarkGetOCIUsage(b *testing.B) {
	if !isConfigured() {
		b.Skip("OCI not configured, skipping benchmark")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := getOCIUsage()
		if err != nil {
			b.Fatalf("getOCIUsage() error = %v", err)
		}
	}
}
