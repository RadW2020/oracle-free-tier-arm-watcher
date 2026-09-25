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
