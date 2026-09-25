package main

import (
	"time"

	"github.com/RadW2020/oracle-free-tier-arm-watcher/internal/freetier"
	"github.com/RadW2020/oracle-free-tier-arm-watcher/internal/snapshot"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// Compute Metrics
	armOCPUsUsed = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "oci_compute_arm_ocpus_used",
		Help: "Number of ARM OCPUs currently in use",
	})
	armOCPUsLimit = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "oci_compute_arm_ocpus_limit",
		Help: "Limit of ARM OCPUs allowed in Free Tier",
	})

	armMemoryUsed = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "oci_compute_arm_memory_gb_used",
		Help: "Amount of ARM Memory (GB) currently in use",
	})
	armMemoryLimit = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "oci_compute_arm_memory_gb_limit",
		Help: "Limit of ARM Memory (GB) allowed in Free Tier",
	})

	amdInstancesUsed = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "oci_compute_amd_instances_used",
		Help: "Number of AMD Micro instances currently in use",
	})
	amdInstancesLimit = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "oci_compute_amd_instances_limit",
		Help: "Limit of AMD Micro instances allowed in Free Tier",
	})

	// Storage Metrics
	blockStorageUsed = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "oci_block_storage_gb_used",
		Help: "Total Block Storage (GB) currently in use (Boot + Block)",
	})
	blockStorageLimit = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "oci_block_storage_gb_limit",
		Help: "Total Block Storage (GB) limit in Free Tier",
	})

	bootVolumesSize = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "oci_storage_boot_volumes_gb",
		Help: "Storage used by Boot Volumes (GB)",
	})
	blockVolumesSize = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "oci_storage_block_volumes_gb",
		Help: "Storage used by Block Volumes (GB)",
	})

	// Object Storage Metrics
	objectStorageUsed = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "oci_object_storage_gb_used",
		Help: "Total Object Storage (GB) currently in use",
	})
	objectStorageLimit = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "oci_object_storage_gb_limit",
		Help: "Object Storage (GB) limit in Free Tier",
	})

	bucketSize = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "oci_object_storage_bucket_gb",
		Help: "Size of an individual Object Storage bucket (GB)",
	}, []string{"bucket_name"})

	// Network Metrics
	publicIPsUsed = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "oci_public_ips_used",
		Help: "Number of Public IPs currently in use",
	})
	publicIPsLimit = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "oci_public_ips_limit",
		Help: "Limit of Public IPs allowed in Free Tier",
	})

	loadBalancersUsed = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "oci_load_balancers_used",
		Help: "Number of Load Balancers currently in use",
	})
	loadBalancersLimit = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "oci_load_balancers_limit",
		Help: "Limit of Load Balancers allowed in Free Tier",
	})

	// Database Metrics
	autonomousDBsUsed = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "oci_database_autonomous_used",
		Help: "Number of Autonomous Databases currently in use",
	})
	autonomousDBsLimit = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "oci_database_autonomous_limit",
		Help: "Limit of Autonomous Databases allowed in Free Tier",
	})

	databaseStorageUsed = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "oci_database_storage_gb_used",
		Help: "Total Storage used by Autonomous Databases (GB)",
	})
	databaseStorageLimit = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "oci_database_storage_gb_limit",
		Help: "Limit of Database Storage allowed in Free Tier",
	})

	// Bandwidth Metrics
	bandwidthEgressUsed = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "oci_bandwidth_egress_gb_used",
		Help: "Egress bandwidth used this month (GB)",
	})
	bandwidthEgressLimit = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "oci_bandwidth_egress_tb_limit",
		Help: "Egress bandwidth limit per month (TB)",
	})
	bandwidthPercentage = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "oci_bandwidth_egress_percentage",
		Help: "Percentage of monthly egress bandwidth used",
	})

	// Saturation Metrics
	//
	// Ninguna de estas consume cuota del Free Tier y por eso no entran en
	// oci_overall_status: ese gauge sigue significando "cuanto te queda antes
	// de pagar". Estas responden a la otra pregunta, la que el 17/09/2026 se
	// quedo sin respuesta durante dos horas: "por que va todo lento".
	instanceCPUPercentage = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "oci_instance_cpu_percentage",
		Help: "CPU utilization of the instance (%), from OCI Monitoring",
	})
	instanceMemoryPercentage = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "oci_instance_memory_percentage",
		Help: "Memory utilization of the instance (%), from OCI Monitoring",
	})
	networkIngressRate = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "oci_network_ingress_mb_per_min",
		Help: "Real inbound traffic on the instance VNIC (MB/min), excludes Docker interfaces",
	})
	networkEgressRate = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "oci_network_egress_mb_per_min",
		Help: "Real outbound traffic on the instance VNIC (MB/min), excludes Docker interfaces",
	})
	networkIngressDropsRate = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "oci_network_ingress_throttle_drops_per_min",
		Help: "Inbound packets dropped by the OCI VNIC shaper in the last measured minute",
	})
	networkIngressDropsHour = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "oci_network_ingress_throttle_drops_1h",
		Help: "Inbound packets dropped by the OCI VNIC shaper over the last hour (>0 means other services are losing SYNs)",
	})
	saturationSampleAge = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "oci_saturation_sample_age_seconds",
		Help: "Age of the newest saturation datapoint: OCI Monitoring publishes with a few minutes of lag",
	})

	// General Status
	overallStatus = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "oci_overall_status",
		Help: "Status of the quota that fills up on its own: object storage, DB storage and egress (0=OK, 1=ATTENTION, 2=WARNING, 3=CRITICAL)",
	})
	allocationPercentage = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "oci_allocation_percentage",
		Help: "Highest percentage among the quotas allocated by design (OCPUs, RAM, block storage, IPs, DBs): 100% is the goal of a well-used Free Tier, not an incident",
	})

	lastUpdateTimestamp = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "oci_watcher_last_update_timestamp",
		Help: "Unix timestamp of the last successful data fetch from OCI",
	})

	// oci_overall_status no sabe de datos que faltan (una fuente caída
	// cuenta como 0 %): este gauge es el que lo dice. Una alerta sobre
	// oci_overall_status sin mirar éste puede estar leyendo ceros de relleno.
	dataComplete = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "oci_data_complete",
		Help: "1 if every OCI source answered in the last read, 0 if some values are placeholders for unknown data",
	})
	ociReadsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "watcher_oci_reads_total",
		Help: "Full reads of the tenancy, by outcome (ok, partial, error)",
	}, []string{"outcome"})
)

// registerSnapshotAge publica la antigüedad de la lectura guardada.
func registerSnapshotAge(store *snapshot.Store) {
	promauto.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "watcher_snapshot_age_seconds",
		Help: "Age of the cached snapshot served by /v1 and MCP (-1 if there is none yet)",
	}, func() float64 {
		snap, ok := store.Current()
		if !ok {
			return -1
		}
		return store.Source().Now().Sub(snap.Reading.ObservedAt).Seconds()
	})
}

// updateMetrics actualiza los valores de Prometheus con una lectura
func updateMetrics(reading freetier.Reading) {
	usage := &reading.Usage

	// Compute
	armOCPUsUsed.Set(usage.Compute.ARM.OCPUs.Used)
	armOCPUsLimit.Set(usage.Compute.ARM.OCPUs.Limit)
	armMemoryUsed.Set(usage.Compute.ARM.MemoryGB.Used)
	armMemoryLimit.Set(usage.Compute.ARM.MemoryGB.Limit)
	amdInstancesUsed.Set(usage.Compute.AMD.Instances.Used)
	amdInstancesLimit.Set(usage.Compute.AMD.Instances.Limit)

	// Storage
	blockStorageUsed.Set(usage.BlockStorage.Total.Used)
	blockStorageLimit.Set(usage.BlockStorage.Total.Limit)
	bootVolumesSize.Set(float64(usage.BlockStorage.BootVolumes.SizeGB))
	blockVolumesSize.Set(float64(usage.BlockStorage.BlockVolumes.SizeGB))

	// Object Storage
	objectStorageUsed.Set(usage.ObjectStorage.Total.Used)
	objectStorageLimit.Set(usage.ObjectStorage.Total.Limit)

	// Limpiar métricas de buckets anteriores (para evitar buckets borrados)
	bucketSize.Reset()
	for _, b := range usage.ObjectStorage.Buckets {
		bucketSize.WithLabelValues(b.Name).Set(b.SizeGB)
	}

	// Network
	publicIPsUsed.Set(usage.PublicIPs.Used)
	publicIPsLimit.Set(usage.PublicIPs.Limit)
	loadBalancersUsed.Set(usage.LoadBalancer.Count.Used)
	loadBalancersLimit.Set(usage.LoadBalancer.Count.Limit)

	// Database
	autonomousDBsUsed.Set(usage.Database.AutonomousDBs.Used)
	autonomousDBsLimit.Set(usage.Database.AutonomousDBs.Limit)
	databaseStorageUsed.Set(usage.Database.StorageUsage.Used)
	databaseStorageLimit.Set(usage.Database.StorageUsage.Limit)

	// Bandwidth
	bandwidthEgressUsed.Set(usage.Bandwidth.EgressGB)
	bandwidthEgressLimit.Set(float64(usage.Bandwidth.LimitTB))
	bandwidthPercentage.Set(float64(usage.Bandwidth.Percentage))

	// Saturación (no entra en el status: ver comentario en la declaración)
	instanceCPUPercentage.Set(usage.Saturation.CPUPercentage)
	instanceMemoryPercentage.Set(usage.Saturation.MemoryPercentage)
	networkIngressRate.Set(usage.Saturation.IngressMBPerMin)
	networkEgressRate.Set(usage.Saturation.EgressMBPerMin)
	networkIngressDropsRate.Set(usage.Saturation.IngressDropsPerMin)
	networkIngressDropsHour.Set(usage.Saturation.IngressDropsLastHour)
	saturationSampleAge.Set(float64(usage.Saturation.SampleAgeSeconds))

	// Estado: lo marca la cuota que se llena sola, no la asignada por
	// diseno. Esta maquina tiene las 4 OCPUs, los 24 GB y los 200 GB de
	// disco al 100 % a proposito, y con el criterio anterior el gauge
	// llevaba meses clavado en CRITICAL: un semaforo siempre en rojo que
	// nadie mira. La logica vive en freetier.Assess para que /usage,
	// /status, la API /v1 y las metricas no puedan discrepar.
	assessment := freetier.Assess(reading)
	allocationPercentage.Set(float64(assessment.AllocationPercentage))

	statusValue := 0.0
	switch assessment.Status {
	case "CRITICAL":
		statusValue = 3.0
	case "WARNING":
		statusValue = 2.0
	case "ATTENTION":
		statusValue = 1.0
	}
	overallStatus.Set(statusValue)

	lastUpdateTimestamp.Set(float64(time.Now().Unix()))

	outcome := "ok"
	if reading.Complete() {
		dataComplete.Set(1)
	} else {
		dataComplete.Set(0)
		outcome = "partial"
	}
	ociReadsTotal.WithLabelValues(outcome).Inc()
}
