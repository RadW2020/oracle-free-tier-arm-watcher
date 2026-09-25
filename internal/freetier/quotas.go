package freetier

import (
	"fmt"
	"math"
)

// QuotaID identifica una cuota en la API /v1. Son los valores válidos del
// parámetro `quota` de get_quota_usage.
type QuotaID string

const (
	QuotaARMOCPUs      QuotaID = "arm_ocpus"
	QuotaARMMemory     QuotaID = "arm_memory"
	QuotaAMDInstances  QuotaID = "amd_instances"
	QuotaBlockStorage  QuotaID = "block_storage"
	QuotaObjectStorage QuotaID = "object_storage"
	QuotaPublicIPs     QuotaID = "public_ips"
	QuotaLoadBalancers QuotaID = "load_balancers"
	QuotaAutonomousDBs QuotaID = "autonomous_databases"
	QuotaDBStorage     QuotaID = "db_storage"
	QuotaEgress        QuotaID = "egress"
)

// Family distingue la cuota asignada por diseño de la que se llena sola.
type Family string

const (
	FamilyAllocated Family = "allocated"
	FamilyAccruing  Family = "accruing"
)

// Quota describe una cuota del Free Tier: de dónde sale, en qué unidad se
// mide y, si es acumulativa, a partir de qué porcentaje avisa.
//
// Es el catálogo que comparten Assess, la API /v1 y el servidor MCP: una
// cuota nueva se añade aquí y aparece en todas partes con el mismo nombre.
type Quota struct {
	ID            QuotaID
	Name          string
	Family        Family
	Unit          string
	Source        string
	WarnThreshold int
	Description   string
	measure       func(u *AllUsage) (used, limit float64)
}

// Measure devuelve el uso y el límite de la cuota.
func (q Quota) Measure(u *AllUsage) (used, limit float64) { return q.measure(u) }

// Percentage devuelve el porcentaje con un decimal. La API heredada trunca a
// entero; aquí 0,9 % no se convierte en 0.
func (q Quota) Percentage(u *AllUsage) float64 {
	used, limit := q.measure(u)
	if limit <= 0 {
		return 0
	}
	return math.Round(used/limit*1000) / 10
}

// legacyPercentage es el porcentaje entero que ya calculaba cada fuente, el
// mismo que publica la API heredada. Assess lo usa para no cambiar ni un
// umbral respecto a assessQuotas.
func (q Quota) legacyPercentage(u *AllUsage) int {
	switch q.ID {
	case QuotaObjectStorage:
		return u.ObjectStorage.Total.Percentage
	case QuotaDBStorage:
		return u.Database.StorageUsage.Percentage
	case QuotaEgress:
		return u.Bandwidth.Percentage
	}
	return int(q.Percentage(u))
}

func (q Quota) warningDetail(u *AllUsage) string {
	if q.ID == QuotaEgress {
		return fmt.Sprintf(" (%.1f GB / %d TB)", u.Bandwidth.EgressGB, u.Bandwidth.LimitTB)
	}
	return ""
}

// Quotas es el catálogo completo, en orden estable.
var Quotas = []Quota{
	{
		ID: QuotaARMOCPUs, Name: "ARM OCPUs", Family: FamilyAllocated, Unit: "ocpu", Source: SourceCompute,
		Description: "OCPUs of RUNNING Ampere A1 instances. STOPPED instances are not counted.",
		measure:     func(u *AllUsage) (float64, float64) { return u.Compute.ARM.OCPUs.Used, Limits.Compute.ARM.OCPUs },
	},
	{
		ID: QuotaARMMemory, Name: "ARM memory", Family: FamilyAllocated, Unit: "GB", Source: SourceCompute,
		Description: "Memory assigned to RUNNING Ampere A1 instances (not how much of it is in use: see memory_percent in the saturation timeline).",
		measure:     func(u *AllUsage) (float64, float64) { return u.Compute.ARM.MemoryGB.Used, Limits.Compute.ARM.MemoryGB },
	},
	{
		ID: QuotaAMDInstances, Name: "AMD Micro instances", Family: FamilyAllocated, Unit: "count", Source: SourceCompute,
		Description: "RUNNING VM.Standard.E2.1.Micro instances.",
		measure: func(u *AllUsage) (float64, float64) {
			return u.Compute.AMD.Instances.Used, float64(Limits.Compute.AMD.MaxInstances)
		},
	},
	{
		ID: QuotaBlockStorage, Name: "Block storage", Family: FamilyAllocated, Unit: "GB", Source: SourceBlockStorage,
		Description: "Boot volumes plus block volumes.",
		measure: func(u *AllUsage) (float64, float64) {
			return u.BlockStorage.Total.Used, float64(Limits.BlockStorage.TotalGB)
		},
	},
	{
		ID: QuotaObjectStorage, Name: "Object Storage", Family: FamilyAccruing, Unit: "GB", Source: SourceObjectStorage, WarnThreshold: 80,
		Description: "Approximate size of all buckets. Grows on its own; going over the limit is billed.",
		measure: func(u *AllUsage) (float64, float64) {
			return u.ObjectStorage.Total.Used, float64(Limits.ObjectStorage.TotalGB)
		},
	},
	{
		ID: QuotaPublicIPs, Name: "Reserved public IPs", Family: FamilyAllocated, Unit: "count", Source: SourcePublicIPs,
		Description: "Reserved public IPs in the region.",
		measure: func(u *AllUsage) (float64, float64) {
			return u.PublicIPs.Used, float64(Limits.PublicIPs.Reserved)
		},
	},
	{
		ID: QuotaLoadBalancers, Name: "Load balancers", Family: FamilyAllocated, Unit: "count", Source: SourceLoadBalancer,
		Description: "Flexible load balancers (the free one is 10 Mbps).",
		measure: func(u *AllUsage) (float64, float64) {
			return u.LoadBalancer.Count.Used, float64(Limits.LoadBalancer.Instances)
		},
	},
	{
		ID: QuotaAutonomousDBs, Name: "Autonomous Databases", Family: FamilyAllocated, Unit: "count", Source: SourceDatabase,
		Description: "Autonomous Databases that are not TERMINATED.",
		measure: func(u *AllUsage) (float64, float64) {
			return u.Database.AutonomousDBs.Used, float64(Limits.Database.AutonomousDBs)
		},
	},
	{
		ID: QuotaDBStorage, Name: "DB Storage", Family: FamilyAccruing, Unit: "GB", Source: SourceDatabase, WarnThreshold: 80,
		Description: "Storage of the Autonomous Databases. Grows on its own; going over the limit is billed.",
		measure: func(u *AllUsage) (float64, float64) {
			return u.Database.StorageUsage.Used, float64(Limits.Database.TotalStorageGB)
		},
	},
	{
		// Bandwidth avisa antes que el resto: 10 TB se van muy deprisa si
		// algo se desmadra, y a 80 % ya no da tiempo a reaccionar.
		ID: QuotaEgress, Name: "Bandwidth", Family: FamilyAccruing, Unit: "GB", Source: SourceBandwidth, WarnThreshold: 50,
		Description: "Outbound traffic to the internet this calendar month (UTC), from oci_vcn VnicToNetworkBytes. Resets on the 1st; going over is billed.",
		measure: func(u *AllUsage) (float64, float64) {
			return u.Bandwidth.EgressGB, float64(Limits.Bandwidth.EgressTBPerMonth) * 1024
		},
	},
}

// QuotaByID busca una cuota del catálogo.
func QuotaByID(id QuotaID) (Quota, bool) {
	for _, q := range Quotas {
		if q.ID == id {
			return q, true
		}
	}
	return Quota{}, false
}

// AccruingQuotas devuelve las cuotas que se llenan solas, en el orden en que
// las evaluaba assessQuotas (object storage, DB storage, bandwidth).
func AccruingQuotas() []Quota {
	var out []Quota
	for _, q := range Quotas {
		if q.Family == FamilyAccruing {
			out = append(out, q)
		}
	}
	return out
}

// Finalize rellena límites y porcentajes a partir de los valores usados.
//
// Las fuentes sólo miden; los porcentajes se calculan aquí, en un único
// sitio, para que la fuente de OCI y la de fixtures no puedan discrepar.
func Finalize(u *AllUsage) {
	u.Compute.ARM.OCPUs = NewMetric(u.Compute.ARM.OCPUs.Used, Limits.Compute.ARM.OCPUs)
	u.Compute.ARM.MemoryGB = NewMetric(u.Compute.ARM.MemoryGB.Used, Limits.Compute.ARM.MemoryGB)
	u.Compute.AMD.Instances = NewMetric(u.Compute.AMD.Instances.Used, float64(Limits.Compute.AMD.MaxInstances))
	u.BlockStorage.Total = NewMetric(u.BlockStorage.Total.Used, float64(Limits.BlockStorage.TotalGB))
	u.ObjectStorage.Total = NewMetric(u.ObjectStorage.Total.Used, float64(Limits.ObjectStorage.TotalGB))
	u.PublicIPs = NewMetric(u.PublicIPs.Used, float64(Limits.PublicIPs.Reserved))
	u.LoadBalancer.Count = NewMetric(u.LoadBalancer.Count.Used, float64(Limits.LoadBalancer.Instances))
	u.Database.AutonomousDBs = NewMetric(u.Database.AutonomousDBs.Used, float64(Limits.Database.AutonomousDBs))
	u.Database.StorageUsage = NewMetric(u.Database.StorageUsage.Used, float64(Limits.Database.TotalStorageGB))
	u.Bandwidth.LimitTB = Limits.Bandwidth.EgressTBPerMonth
	u.Bandwidth.Percentage = 0
	if limitGB := u.Bandwidth.LimitGB(); limitGB > 0 {
		u.Bandwidth.Percentage = int((u.Bandwidth.EgressGB / limitGB) * 100)
	}
	if u.ObjectStorage.Buckets == nil {
		u.ObjectStorage.Buckets = []BucketInfo{}
	}
	if u.LoadBalancer.LoadBalancers == nil {
		u.LoadBalancer.LoadBalancers = []LoadBalancerInfo{}
	}
}
