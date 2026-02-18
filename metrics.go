package main

import (
	"time"

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

	// General Status
	overallStatus = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "oci_overall_status",
		Help: "Overall status of OCI resources (0=OK, 1=ATTENTION, 2=WARNING, 3=CRITICAL)",
	})

	lastUpdateTimestamp = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "oci_watcher_last_update_timestamp",
		Help: "Unix timestamp of the last successful data fetch from OCI",
	})
)

// updateMetrics actualiza los valores de Prometheus con los datos de AllUsage
func updateMetrics(usage *AllUsage) {
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

	// Calcular status numérico
	maxPercent := 0
	percentages := []int{
		usage.Compute.ARM.OCPUs.Percentage,
		usage.Compute.ARM.MemoryGB.Percentage,
		usage.Compute.AMD.Instances.Percentage,
		usage.BlockStorage.Total.Percentage,
		usage.ObjectStorage.Total.Percentage,
		usage.PublicIPs.Percentage,
		usage.Database.AutonomousDBs.Percentage,
		usage.Database.StorageUsage.Percentage,
	}
	for _, p := range percentages {
		if p > maxPercent {
			maxPercent = p
		}
	}

	statusValue := 0.0
	if maxPercent >= 90 {
		statusValue = 3.0 // CRITICAL
	} else if maxPercent >= 80 {
		statusValue = 2.0 // WARNING
	} else if maxPercent >= 60 {
		statusValue = 1.0 // ATTENTION
	}
	overallStatus.Set(statusValue)

	lastUpdateTimestamp.Set(float64(time.Now().Unix()))
}

// BackgroundMetricsWorker inicia un loop que actualiza las métricas periódicamente
func BackgroundMetricsWorker(interval time.Duration) {
	logger.Info().Dur("interval", interval).Msg("Starting background metrics worker")
	
	// Primera ejecución inmediata
	runUpdate()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		runUpdate()
	}
}

func runUpdate() {
	if !isConfigured() {
		logger.Warn().Msg("Metrics worker: OCI not configured, skipping update")
		return
	}

	logger.Debug().Msg("Metrics worker: Fetching OCI usage...")
	usage, err := getOCIUsage()
	if err != nil {
		logger.Error().Err(err).Msg("Metrics worker: Error fetching OCI usage")
		return
	}

	updateMetrics(usage)
	logger.Info().Msg("Metrics worker: Successfully updated Prometheus metrics")
}
