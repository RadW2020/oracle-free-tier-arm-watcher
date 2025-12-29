// Package main es el punto de entrada de la aplicación
// En Go, cada programa ejecutable debe tener un package main y una función main()
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/joho/godotenv"
)

// FreeTierLimits define los límites de la capa gratuita de Oracle Cloud
// En Go, los structs son como clases pero sin métodos incorporados
// Los campos con mayúscula son públicos, con minúscula son privados
type FreeTierLimits struct {
	Compute struct {
		ARM struct {
			OCPUs        float64 `json:"ocpus"`
			MemoryGB     float64 `json:"memoryGB"`
			MaxInstances int     `json:"maxInstances"`
		} `json:"arm"`
		AMD struct {
			OCPUs        float64 `json:"ocpus"`
			MemoryGB     float64 `json:"memoryGB"`
			MaxInstances int     `json:"maxInstances"`
		} `json:"amd"`
	} `json:"compute"`
	BlockStorage struct {
		TotalGB int `json:"totalGB"`
	} `json:"blockStorage"`
	ObjectStorage struct {
		TotalGB          int `json:"totalGB"`
		RequestsPerMonth int `json:"requestsPerMonth"`
	} `json:"objectStorage"`
	Bandwidth struct {
		EgressTBPerMonth int `json:"egressTBPerMonth"`
	} `json:"bandwidth"`
	Database struct {
		AutonomousDBs  int `json:"autonomousDBs"`
		TotalStorageGB int `json:"totalStorageGB"`
	} `json:"database"`
	LoadBalancer struct {
		Instances    int `json:"instances"`
		BandwidthMbps int `json:"bandwidthMbps"`
	} `json:"loadBalancer"`
}

// Limits contiene los valores de la Free Tier de Oracle Cloud
// Esta es una variable global (a nivel de paquete)
var Limits = FreeTierLimits{}

// init() se ejecuta automáticamente antes de main()
// Es útil para inicializar variables globales
func init() {
	// Inicializar los límites de Free Tier
	Limits.Compute.ARM.OCPUs = 4
	Limits.Compute.ARM.MemoryGB = 24
	Limits.Compute.ARM.MaxInstances = 4
	Limits.Compute.AMD.OCPUs = 0.25
	Limits.Compute.AMD.MemoryGB = 1
	Limits.Compute.AMD.MaxInstances = 2
	Limits.BlockStorage.TotalGB = 200
	Limits.ObjectStorage.TotalGB = 10
	Limits.ObjectStorage.RequestsPerMonth = 50000
	Limits.Bandwidth.EgressTBPerMonth = 10
	Limits.Database.AutonomousDBs = 2
	Limits.Database.TotalStorageGB = 20
	Limits.LoadBalancer.Instances = 1
	Limits.LoadBalancer.BandwidthMbps = 10
}

// UsageMetric representa una métrica de uso individual
type UsageMetric struct {
	Used       float64 `json:"used"`
	Limit      float64 `json:"limit"`
	Percentage int     `json:"percentage"`
}

// ComputeUsage contiene el uso de compute
type ComputeUsage struct {
	ARM struct {
		OCPUs     UsageMetric `json:"ocpus"`
		MemoryGB  UsageMetric `json:"memoryGB"`
		Instances int         `json:"instances"`
	} `json:"arm"`
	AMD struct {
		Instances UsageMetric `json:"instances"`
	} `json:"amd"`
	TotalInstances int    `json:"totalInstances"`
	Error          string `json:"error,omitempty"`
}

// StorageUsage contiene el uso de almacenamiento
type StorageUsage struct {
	BootVolumes struct {
		Count  int `json:"count"`
		SizeGB int `json:"sizeGB"`
	} `json:"bootVolumes"`
	BlockVolumes struct {
		Count  int `json:"count"`
		SizeGB int `json:"sizeGB"`
	} `json:"blockVolumes"`
	Total UsageMetric `json:"total"`
	Error string      `json:"error,omitempty"`
}

// ObjectStorageUsage contiene el uso de object storage
type ObjectStorageUsage struct {
	Buckets []BucketInfo `json:"buckets"`
	Total   UsageMetric  `json:"total"`
	Error   string       `json:"error,omitempty"`
}

// BucketInfo contiene info de un bucket
type BucketInfo struct {
	Name   string  `json:"name"`
	SizeGB float64 `json:"sizeGB"`
}

// LoadBalancerUsage contiene el uso de load balancers
type LoadBalancerUsage struct {
	Count         UsageMetric        `json:"count"`
	LoadBalancers []LoadBalancerInfo `json:"loadBalancers"`
	Error         string             `json:"error,omitempty"`
}

// LoadBalancerInfo contiene info de un load balancer
type LoadBalancerInfo struct {
	Name  string `json:"name"`
	Shape string `json:"shape"`
	State string `json:"state"`
}

// AllUsage contiene todo el uso
type AllUsage struct {
	Compute       ComputeUsage       `json:"compute"`
	BlockStorage  StorageUsage       `json:"blockStorage"`
	PublicIPs     UsageMetric        `json:"publicIPs"`
	ObjectStorage ObjectStorageUsage `json:"objectStorage"`
	LoadBalancer  LoadBalancerUsage  `json:"loadBalancer"`
}

// UsageResponse es la respuesta del endpoint /usage
type UsageResponse struct {
	Status             string         `json:"status"`
	MaxUsagePercentage int            `json:"maxUsagePercentage"`
	Warnings           []string       `json:"warnings"`
	Timestamp          string         `json:"timestamp"`
	Configured         bool           `json:"configured"`
	Usage              *AllUsage      `json:"usage,omitempty"`
	FreeTierLimits     FreeTierLimits `json:"freeTierLimits"`
	Error              string         `json:"error,omitempty"`
	Message            string         `json:"message,omitempty"`
}

// HealthResponse es la respuesta del endpoint /health
type HealthResponse struct {
	Status    string `json:"status"`
	Timestamp string `json:"timestamp"`
}

// StatusResponse es la respuesta del endpoint /status
type StatusResponse struct {
	Status             string   `json:"status"`
	MaxUsagePercentage int      `json:"maxUsagePercentage,omitempty"`
	Warnings           []string `json:"warnings,omitempty"`
	Timestamp          string   `json:"timestamp"`
	Message            string   `json:"message,omitempty"`
}

// LimitsResponse es la respuesta del endpoint /limits
type LimitsResponse struct {
	FreeTierLimits FreeTierLimits `json:"freeTierLimits"`
	Timestamp      string         `json:"timestamp"`
}

// getEnv obtiene una variable de entorno con un valor por defecto
// Esta es una función helper muy común en Go
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// isConfigured verifica si las credenciales de OCI están configuradas
func isConfigured() bool {
	required := []string{
		"OCI_TENANCY_ID",
		"OCI_USER_ID",
		"OCI_FINGERPRINT",
		"OCI_PRIVATE_KEY_PATH",
		"OCI_REGION",
	}
	for _, key := range required {
		if os.Getenv(key) == "" {
			return false
		}
	}
	return true
}

// writeJSON escribe una respuesta JSON
// En Go, las funciones pueden devolver múltiples valores
func writeJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	// json.NewEncoder es más eficiente que json.Marshal para HTTP
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("Error encoding JSON: %v", err)
	}
}

// healthHandler maneja GET /health
// Los handlers en Go reciben (ResponseWriter, *Request)
func healthHandler(w http.ResponseWriter, r *http.Request) {
	// Solo permitir GET
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	writeJSON(w, http.StatusOK, HealthResponse{
		Status:    "ok",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
}

// limitsHandler maneja GET /limits
func limitsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	writeJSON(w, http.StatusOK, LimitsResponse{
		FreeTierLimits: Limits,
		Timestamp:      time.Now().UTC().Format(time.RFC3339),
	})
}

// usageHandler maneja GET /usage
func usageHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !isConfigured() {
		writeJSON(w, http.StatusOK, UsageResponse{
			Status:         "NOT_CONFIGURED",
			Configured:     false,
			Timestamp:      time.Now().UTC().Format(time.RFC3339),
			Error:          "OCI not configured",
			Message:        "Please configure your OCI credentials in the .env file",
			FreeTierLimits: Limits,
		})
		return
	}

	// Obtener uso real de OCI
	usage, err := getOCIUsage()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, UsageResponse{
			Status:         "ERROR",
			Configured:     true,
			Timestamp:      time.Now().UTC().Format(time.RFC3339),
			Error:          err.Error(),
			FreeTierLimits: Limits,
		})
		return
	}

	// Calcular estado general
	percentages := []int{}
	warnings := []string{}

	if usage.Compute.ARM.OCPUs.Percentage > 0 {
		percentages = append(percentages, usage.Compute.ARM.OCPUs.Percentage)
		if usage.Compute.ARM.OCPUs.Percentage >= 80 {
			warnings = append(warnings, fmt.Sprintf("ARM OCPUs at %d%%", usage.Compute.ARM.OCPUs.Percentage))
		}
	}
	if usage.Compute.ARM.MemoryGB.Percentage > 0 {
		percentages = append(percentages, usage.Compute.ARM.MemoryGB.Percentage)
		if usage.Compute.ARM.MemoryGB.Percentage >= 80 {
			warnings = append(warnings, fmt.Sprintf("ARM Memory at %d%%", usage.Compute.ARM.MemoryGB.Percentage))
		}
	}
	if usage.BlockStorage.Total.Percentage > 0 {
		percentages = append(percentages, usage.BlockStorage.Total.Percentage)
		if usage.BlockStorage.Total.Percentage >= 80 {
			warnings = append(warnings, fmt.Sprintf("Block Storage at %d%%", usage.BlockStorage.Total.Percentage))
		}
	}
	if usage.PublicIPs.Percentage > 0 {
		percentages = append(percentages, usage.PublicIPs.Percentage)
		if usage.PublicIPs.Percentage >= 80 {
			warnings = append(warnings, fmt.Sprintf("Public IPs at %d%%", usage.PublicIPs.Percentage))
		}
	}
	if usage.ObjectStorage.Total.Percentage > 0 {
		percentages = append(percentages, usage.ObjectStorage.Total.Percentage)
		if usage.ObjectStorage.Total.Percentage >= 80 {
			warnings = append(warnings, fmt.Sprintf("Object Storage at %d%%", usage.ObjectStorage.Total.Percentage))
		}
	}

	maxPercentage := 0
	for _, p := range percentages {
		if p > maxPercentage {
			maxPercentage = p
		}
	}

	status := "OK"
	if maxPercentage >= 90 {
		status = "CRITICAL"
	} else if maxPercentage >= 80 {
		status = "WARNING"
	} else if maxPercentage >= 60 {
		status = "ATTENTION"
	}

	writeJSON(w, http.StatusOK, UsageResponse{
		Status:             status,
		MaxUsagePercentage: maxPercentage,
		Warnings:           warnings,
		Timestamp:          time.Now().UTC().Format(time.RFC3339),
		Configured:         true,
		Usage:              usage,
		FreeTierLimits:     Limits,
	})
}

// statusHandler maneja GET /status (versión simplificada)
func statusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !isConfigured() {
		writeJSON(w, http.StatusServiceUnavailable, StatusResponse{
			Status:    "NOT_CONFIGURED",
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Message:   "OCI credentials not configured",
		})
		return
	}

	usage, err := getOCIUsage()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, StatusResponse{
			Status:    "ERROR",
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Message:   err.Error(),
		})
		return
	}

	// Calcular estado
	percentages := []int{
		usage.Compute.ARM.OCPUs.Percentage,
		usage.Compute.ARM.MemoryGB.Percentage,
		usage.BlockStorage.Total.Percentage,
		usage.ObjectStorage.Total.Percentage,
		usage.PublicIPs.Percentage,
	}

	maxPercentage := 0
	warnings := []string{}
	for _, p := range percentages {
		if p > maxPercentage {
			maxPercentage = p
		}
		if p >= 80 {
			warnings = append(warnings, fmt.Sprintf("Resource at %d%%", p))
		}
	}

	status := "OK"
	if maxPercentage >= 90 {
		status = "CRITICAL"
	} else if maxPercentage >= 80 {
		status = "WARNING"
	} else if maxPercentage >= 60 {
		status = "ATTENTION"
	}

	writeJSON(w, http.StatusOK, StatusResponse{
		Status:             status,
		MaxUsagePercentage: maxPercentage,
		Warnings:           warnings,
		Timestamp:          time.Now().UTC().Format(time.RFC3339),
	})
}

// main es el punto de entrada del programa
func main() {
	// Cargar variables de entorno desde .env
	// El _ ignora el error (común si el archivo no existe)
	_ = godotenv.Load()

	port := getEnv("PORT", "3000")

	// Registrar los handlers
	// http.HandleFunc asocia una ruta con una función
	http.HandleFunc("/health", healthHandler)
	http.HandleFunc("/limits", limitsHandler)
	http.HandleFunc("/usage", usageHandler)
	http.HandleFunc("/status", statusHandler)

	// Imprimir información de inicio
	fmt.Println("🔍 Oracle Free Tier Watcher running on port", port)
	fmt.Printf("📊 Usage endpoint: http://localhost:%s/usage\n", port)
	fmt.Printf("💚 Health check: http://localhost:%s/health\n", port)
	fmt.Printf("📋 Limits info: http://localhost:%s/limits\n", port)
	fmt.Printf("⚡ Quick status: http://localhost:%s/status\n", port)

	// Iniciar el servidor
	// log.Fatal termina el programa si hay un error
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
