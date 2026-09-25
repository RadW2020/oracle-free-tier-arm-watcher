// Package freetier contiene las reglas de dominio del watcher: los límites
// de la Always Free, los tipos de uso y cómo se decide si hay que preocuparse.
//
// No hace I/O. Las fuentes de datos (internal/source) rellenan estos tipos y
// todo lo demás —los endpoints heredados, la API /v1, el servidor MCP y las
// métricas de Prometheus— los lee a través de estas funciones, de modo que no
// pueden discrepar sobre qué significa un número.
package freetier

import "time"

// FreeTierLimits define los límites de la capa gratuita de Oracle Cloud.
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
		Instances     int `json:"instances"`
		BandwidthMbps int `json:"bandwidthMbps"`
	} `json:"loadBalancer"`
	PublicIPs struct {
		Reserved int `json:"reserved"`
	} `json:"publicIPs"`
}

// Limits contiene los valores de la Free Tier de Oracle Cloud.
var Limits = defaultLimits()

func defaultLimits() FreeTierLimits {
	var l FreeTierLimits
	l.Compute.ARM.OCPUs = 4
	l.Compute.ARM.MemoryGB = 24
	l.Compute.ARM.MaxInstances = 4
	l.Compute.AMD.OCPUs = 0.25
	l.Compute.AMD.MemoryGB = 1
	l.Compute.AMD.MaxInstances = 2
	l.BlockStorage.TotalGB = 200
	l.ObjectStorage.TotalGB = 10
	l.ObjectStorage.RequestsPerMonth = 50000
	l.Bandwidth.EgressTBPerMonth = 10
	l.Database.AutonomousDBs = 2
	l.Database.TotalStorageGB = 20
	l.LoadBalancer.Instances = 1
	l.LoadBalancer.BandwidthMbps = 10
	l.PublicIPs.Reserved = 2
	return l
}

// UsageMetric representa una métrica de uso individual.
type UsageMetric struct {
	Used       float64 `json:"used"`
	Limit      float64 `json:"limit"`
	Percentage int     `json:"percentage"`
}

// NewMetric construye una métrica con su porcentaje.
//
// El porcentaje se trunca a entero porque así lo publica la API heredada y
// los checks de Checkly comparan contra enteros. La API /v1 calcula el suyo
// con decimales a partir de Used y Limit.
func NewMetric(used, limit float64) UsageMetric {
	m := UsageMetric{Used: used, Limit: limit}
	if limit > 0 {
		m.Percentage = int((used / limit) * 100)
	}
	return m
}

// ComputeUsage contiene el uso de compute.
type ComputeUsage struct {
	ARM struct {
		OCPUs     UsageMetric `json:"ocpus"`
		MemoryGB  UsageMetric `json:"memoryGB"`
		Instances int         `json:"instances"`
	} `json:"arm"`
	AMD struct {
		Instances UsageMetric `json:"instances"`
	} `json:"amd"`
	TotalInstances int `json:"totalInstances"`
	// Detalle por instancia. Aditivo respecto al contrato heredado: sirve
	// para contestar "qué está usando las OCPUs" sin ir a la consola.
	InstanceDetails []InstanceInfo `json:"instanceDetails,omitempty"`
	Error           string         `json:"error,omitempty"`
}

// InstanceInfo describe una instancia en ejecución.
type InstanceInfo struct {
	Name     string  `json:"name"`
	Shape    string  `json:"shape"`
	OCPUs    float64 `json:"ocpus"`
	MemoryGB float64 `json:"memoryGB"`
	State    string  `json:"state"`
}

// StorageUsage contiene el uso de almacenamiento.
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
	// Detalle por volumen, aditivo como InstanceDetails.
	Volumes []VolumeInfo `json:"volumes,omitempty"`
	Error   string       `json:"error,omitempty"`
}

// VolumeInfo describe un boot volume o un block volume.
type VolumeInfo struct {
	Name   string `json:"name"`
	Kind   string `json:"kind"` // "boot" | "block"
	SizeGB int    `json:"sizeGB"`
	State  string `json:"state"`
}

// ObjectStorageUsage contiene el uso de object storage.
type ObjectStorageUsage struct {
	Buckets []BucketInfo `json:"buckets"`
	Total   UsageMetric  `json:"total"`
	Error   string       `json:"error,omitempty"`
}

// BucketInfo contiene info de un bucket.
//
// SizeGB vale -1 cuando no se pudo leer el tamaño: es el contrato heredado y
// se mantiene. SizeKnown lo dice sin centinelas, que es lo que lee la API /v1.
type BucketInfo struct {
	Name      string  `json:"name"`
	SizeGB    float64 `json:"sizeGB"`
	SizeKnown bool    `json:"sizeKnown"`
}

// LoadBalancerUsage contiene el uso de load balancers.
type LoadBalancerUsage struct {
	Count         UsageMetric        `json:"count"`
	LoadBalancers []LoadBalancerInfo `json:"loadBalancers"`
	Error         string             `json:"error,omitempty"`
}

// LoadBalancerInfo contiene info de un load balancer.
type LoadBalancerInfo struct {
	Name  string `json:"name"`
	Shape string `json:"shape"`
	State string `json:"state"`
}

// DatabaseUsage contiene el uso de bases de datos.
type DatabaseUsage struct {
	AutonomousDBs UsageMetric `json:"autonomousDBs"`
	StorageUsage  UsageMetric `json:"storageUsage"`
	Count         int         `json:"count"`
	Error         string      `json:"error,omitempty"`
}

// BandwidthUsage contiene el uso de transferencia.
type BandwidthUsage struct {
	EgressGB   float64 `json:"egressGB"`
	LimitTB    int     `json:"limitTB"`
	Percentage int     `json:"percentage"`
	Error      string  `json:"error,omitempty"`
}

// LimitGB devuelve el tope mensual de egress en GB (2^30 bytes, como EgressGB).
func (b BandwidthUsage) LimitGB() float64 { return float64(b.LimitTB) * 1024 }

// SaturationUsage recoge senales de saturacion del host.
//
// Ninguna de estas metricas consume cuota del Free Tier —el trafico de
// entrada ni siquiera se factura—, y por eso NO entran en el calculo de
// status ni en maxUsagePercentage: estan aqui porque son las que explican
// una caida, no las que anuncian una factura.
//
// El incidente del 17/09/2026 se vio exactamente asi: 379 MB/min de entrada
// sostenidos y ~81.000 paquetes descartados por el shaper de OCI en seis
// minutos, mientras todas las cuotas seguian en verde y el watcher informaba
// OK con toda la razon. Ver POSTMORTEM-2026-09-17.md.
type SaturationUsage struct {
	CPUPercentage float64 `json:"cpuPercentage"`
	// Memoria de la instancia: sin ella no se puede contestar si cabe un
	// servicio más en la máquina, que es la única capacidad que queda cuando
	// la cuota está asignada entera.
	MemoryPercentage     float64 `json:"memoryPercentage"`
	IngressMBPerMin      float64 `json:"ingressMBPerMin"`
	EgressMBPerMin       float64 `json:"egressMBPerMin"`
	IngressDropsPerMin   float64 `json:"ingressThrottleDropsPerMin"`
	IngressDropsLastHour float64 `json:"ingressThrottleDropsLastHour"`
	// Antiguedad del ultimo dato disponible: la API de Monitoring publica
	// con unos minutos de retraso, asi que un valor de 0 seria mentira.
	SampleAgeSeconds int `json:"sampleAgeSeconds"`
	// Señales sin ningún dato en la ventana. Su valor de arriba es 0 porque
	// no hay otro que poner, no porque se haya medido un 0.
	MissingSignals []string `json:"missingSignals,omitempty"`
	Error          string   `json:"error,omitempty"`
}

// AllUsage contiene todo el uso.
type AllUsage struct {
	Compute       ComputeUsage       `json:"compute"`
	BlockStorage  StorageUsage       `json:"blockStorage"`
	PublicIPs     UsageMetric        `json:"publicIPs"`
	ObjectStorage ObjectStorageUsage `json:"objectStorage"`
	LoadBalancer  LoadBalancerUsage  `json:"loadBalancer"`
	Database      DatabaseUsage      `json:"database"`
	Bandwidth     BandwidthUsage     `json:"bandwidth"`
	Saturation    SaturationUsage    `json:"saturation"`
}

// Nombres de las fuentes de datos. Cada una es una llamada (o un grupo de
// llamadas) a OCI que puede fallar por separado.
const (
	SourceCompute       = "compute"
	SourceBlockStorage  = "blockStorage"
	SourceObjectStorage = "objectStorage"
	SourceLoadBalancer  = "loadBalancer"
	SourcePublicIPs     = "publicIPs"
	SourceDatabase      = "database"
	SourceBandwidth     = "bandwidth"
	SourceSaturation    = "saturation"
)

// AllSources enumera las fuentes en un orden estable.
var AllSources = []string{
	SourceCompute, SourceBlockStorage, SourceObjectStorage, SourceLoadBalancer,
	SourcePublicIPs, SourceDatabase, SourceBandwidth, SourceSaturation,
}

// SourceStatus dice si una fuente respondió y, si no, por qué.
type SourceStatus struct {
	Name      string `json:"name"`
	Available bool   `json:"available"`
	ErrorCode string `json:"errorCode,omitempty"`
	Message   string `json:"message,omitempty"`
}

// Reading es una lectura completa de la tenancy: el uso, el estado de cada
// fuente y cuándo se tomó.
//
// Existe para que "no se sabe" deje de ser indistinguible de "cero". Antes,
// una consulta de bandwidth fallida dejaba el porcentaje a 0 y el estado en
// OK; ahora la lectura dice explícitamente que esa fuente no está disponible.
type Reading struct {
	Usage      AllUsage       `json:"usage"`
	Sources    []SourceStatus `json:"sources"`
	ObservedAt time.Time      `json:"observedAt"`
}

// NewReading construye una lectura en la que todas las fuentes respondieron
// salvo las de failures.
func NewReading(usage AllUsage, observedAt time.Time, failures map[string]SourceStatus) Reading {
	r := Reading{Usage: usage, ObservedAt: observedAt.UTC(), Sources: make([]SourceStatus, 0, len(AllSources))}
	for _, name := range AllSources {
		if f, failed := failures[name]; failed {
			f.Name = name
			f.Available = false
			r.Sources = append(r.Sources, f)
			continue
		}
		r.Sources = append(r.Sources, SourceStatus{Name: name, Available: true})
	}
	return r
}

// Available dice si una fuente respondió. Una fuente que no aparece en
// Sources se considera no disponible: sin noticias no son buenas noticias.
func (r Reading) Available(name string) bool {
	for _, s := range r.Sources {
		if s.Name == name {
			return s.Available
		}
	}
	return false
}

// Status devuelve el estado de una fuente.
func (r Reading) Status(name string) SourceStatus {
	for _, s := range r.Sources {
		if s.Name == name {
			return s
		}
	}
	return SourceStatus{Name: name, Available: false, ErrorCode: "unknown", Message: "source not reported"}
}

// Complete dice si todas las fuentes respondieron.
func (r Reading) Complete() bool {
	for _, name := range AllSources {
		if !r.Available(name) {
			return false
		}
	}
	return true
}
