// Package main - Este archivo contiene la lógica para conectar con OCI
package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/oracle/oci-go-sdk/v65/loadbalancer"
	"github.com/oracle/oci-go-sdk/v65/objectstorage"
)

// createConfigProvider crea el proveedor de autenticación de OCI
// En Go, los errores se devuelven como segundo valor (no se lanzan excepciones)
func createConfigProvider() (common.ConfigurationProvider, error) {
	tenancy := os.Getenv("OCI_TENANCY_ID")
	user := os.Getenv("OCI_USER_ID")
	fingerprint := os.Getenv("OCI_FINGERPRINT")
	privateKeyPath := os.Getenv("OCI_PRIVATE_KEY_PATH")
	region := os.Getenv("OCI_REGION")

	// Leer la clave privada desde el archivo
	privateKeyBytes, err := os.ReadFile(privateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("error reading private key: %w", err)
	}

	// Crear el proveedor de configuración
	// common.NewRawConfigurationProvider es una función del SDK de OCI
	provider := common.NewRawConfigurationProvider(
		tenancy,
		user,
		region,
		fingerprint,
		string(privateKeyBytes),
		nil, // passphrase (nil si no tiene)
	)

	return provider, nil
}

// getCompartmentID obtiene el ID del compartimento a monitorear
func getCompartmentID() string {
	compartmentID := os.Getenv("OCI_COMPARTMENT_ID")
	if compartmentID == "" {
		// Si no hay compartimento específico, usar el tenancy (root)
		return os.Getenv("OCI_TENANCY_ID")
	}
	return compartmentID
}

// getOCIUsage obtiene todo el uso de OCI de forma paralela
// Usa goroutines para hacer las llamadas a la API de OCI concurrentemente
func getOCIUsage() (*AllUsage, error) {
	provider, err := createConfigProvider()
	if err != nil {
		return nil, err
	}

	compartmentID := getCompartmentID()

	// Usar goroutines para obtener datos en paralelo
	// Esto reduce el tiempo de respuesta significativamente
	var (
		computeUsage       ComputeUsage
		blockStorageUsage  StorageUsage
		objectStorageUsage ObjectStorageUsage
		loadBalancerUsage  LoadBalancerUsage
		publicIPUsage      UsageMetric
	)

	// Canal para sincronización (esperamos 5 goroutines)
	done := make(chan bool, 5)

	// Lanzar todas las consultas en paralelo
	go func() {
		computeUsage = getComputeUsage(provider, compartmentID)
		done <- true
	}()

	go func() {
		blockStorageUsage = getBlockStorageUsage(provider, compartmentID)
		done <- true
	}()

	go func() {
		objectStorageUsage = getObjectStorageUsage(provider, compartmentID)
		done <- true
	}()

	go func() {
		loadBalancerUsage = getLoadBalancerUsage(provider, compartmentID)
		done <- true
	}()

	go func() {
		publicIPUsage = getPublicIPsUsage(provider, compartmentID)
		done <- true
	}()

	// Esperar a que todas las goroutines terminen
	for i := 0; i < 5; i++ {
		<-done
	}

	return &AllUsage{
		Compute:       computeUsage,
		BlockStorage:  blockStorageUsage,
		PublicIPs:     publicIPUsage,
		ObjectStorage: objectStorageUsage,
		LoadBalancer:  loadBalancerUsage,
	}, nil
}

// getPublicIPsUsage monitoriza las IPs públicas reservadas (límite free tier: 2)
func getPublicIPsUsage(provider common.ConfigurationProvider, compartmentID string) UsageMetric {
	usage := UsageMetric{Limit: 2} // Límite estándar de Free Tier

	client, err := core.NewVirtualNetworkClientWithConfigurationProvider(provider)
	if err != nil {
		return usage
	}

	request := core.ListPublicIpsRequest{
		CompartmentId: common.String(compartmentID),
		Scope:         core.ListPublicIpsScopeRegion,
	}

	response, err := client.ListPublicIps(context.Background(), request)
	if err != nil {
		return usage
	}

	count := len(response.Items)
	usage.Used = float64(count)
	usage.Percentage = int((usage.Used / usage.Limit) * 100)

	return usage
}

// getComputeUsage obtiene el uso de compute
func getComputeUsage(provider common.ConfigurationProvider, compartmentID string) ComputeUsage {
	usage := ComputeUsage{}

	// Crear cliente de Compute
	client, err := core.NewComputeClientWithConfigurationProvider(provider)
	if err != nil {
		usage.Error = err.Error()
		return usage
	}

	// Listar instancias en ejecución
	// En Go, los parámetros de request suelen ser structs
	request := core.ListInstancesRequest{
		CompartmentId:  common.String(compartmentID),
		LifecycleState: core.InstanceLifecycleStateRunning,
	}

	response, err := client.ListInstances(context.Background(), request)
	if err != nil {
		usage.Error = err.Error()
		return usage
	}

	// Procesar las instancias
	var armOCPUs, armMemoryGB float64
	var armCount, amdCount int

	for _, instance := range response.Items {
		shape := *instance.Shape

		// Detectar si es ARM (Ampere) o AMD
		if strings.Contains(shape, "A1") || strings.Contains(shape, "Ampere") {
			if instance.ShapeConfig != nil {
				if instance.ShapeConfig.Ocpus != nil {
					armOCPUs += float64(*instance.ShapeConfig.Ocpus)
				}
				if instance.ShapeConfig.MemoryInGBs != nil {
					armMemoryGB += float64(*instance.ShapeConfig.MemoryInGBs)
				}
			}
			armCount++
		} else if strings.Contains(shape, "Micro") || strings.Contains(shape, "E2.1.Micro") {
			amdCount++
		}
	}

	// Calcular porcentajes
	usage.ARM.OCPUs = UsageMetric{
		Used:       armOCPUs,
		Limit:      Limits.Compute.ARM.OCPUs,
		Percentage: int((armOCPUs / Limits.Compute.ARM.OCPUs) * 100),
	}
	usage.ARM.MemoryGB = UsageMetric{
		Used:       armMemoryGB,
		Limit:      Limits.Compute.ARM.MemoryGB,
		Percentage: int((armMemoryGB / Limits.Compute.ARM.MemoryGB) * 100),
	}
	usage.ARM.Instances = armCount
	usage.AMD.Instances = UsageMetric{
		Used:       float64(amdCount),
		Limit:      float64(Limits.Compute.AMD.MaxInstances),
		Percentage: int((float64(amdCount) / float64(Limits.Compute.AMD.MaxInstances)) * 100),
	}
	usage.TotalInstances = len(response.Items)

	return usage
}

// getBlockStorageUsage obtiene el uso de block storage
func getBlockStorageUsage(provider common.ConfigurationProvider, compartmentID string) StorageUsage {
	usage := StorageUsage{}

	client, err := core.NewBlockstorageClientWithConfigurationProvider(provider)
	if err != nil {
		usage.Error = err.Error()
		return usage
	}

	// Obtener boot volumes
	bootRequest := core.ListBootVolumesRequest{
		CompartmentId: common.String(compartmentID),
	}
	bootResponse, err := client.ListBootVolumes(context.Background(), bootRequest)
	if err != nil {
		usage.Error = err.Error()
		return usage
	}

	var bootVolumeGB int64
	for _, vol := range bootResponse.Items {
		if vol.SizeInGBs != nil {
			bootVolumeGB += *vol.SizeInGBs
		}
	}
	usage.BootVolumes.Count = len(bootResponse.Items)
	usage.BootVolumes.SizeGB = int(bootVolumeGB)

	// Obtener block volumes
	blockRequest := core.ListVolumesRequest{
		CompartmentId: common.String(compartmentID),
	}
	blockResponse, err := client.ListVolumes(context.Background(), blockRequest)
	if err != nil {
		usage.Error = err.Error()
		return usage
	}

	var blockVolumeGB int64
	for _, vol := range blockResponse.Items {
		if vol.SizeInGBs != nil {
			blockVolumeGB += *vol.SizeInGBs
		}
	}
	usage.BlockVolumes.Count = len(blockResponse.Items)
	usage.BlockVolumes.SizeGB = int(blockVolumeGB)

	// Total
	totalGB := int(bootVolumeGB + blockVolumeGB)
	usage.Total = UsageMetric{
		Used:       float64(totalGB),
		Limit:      float64(Limits.BlockStorage.TotalGB),
		Percentage: int((float64(totalGB) / float64(Limits.BlockStorage.TotalGB)) * 100),
	}

	return usage
}

// getObjectStorageUsage obtiene el uso de object storage
func getObjectStorageUsage(provider common.ConfigurationProvider, compartmentID string) ObjectStorageUsage {
	usage := ObjectStorageUsage{}

	client, err := objectstorage.NewObjectStorageClientWithConfigurationProvider(provider)
	if err != nil {
		usage.Error = err.Error()
		return usage
	}

	// Obtener namespace (requerido para object storage)
	nsRequest := objectstorage.GetNamespaceRequest{}
	nsResponse, err := client.GetNamespace(context.Background(), nsRequest)
	if err != nil {
		usage.Error = err.Error()
		return usage
	}
	namespace := *nsResponse.Value

	// Listar buckets
	bucketsRequest := objectstorage.ListBucketsRequest{
		NamespaceName: common.String(namespace),
		CompartmentId: common.String(compartmentID),
	}
	bucketsResponse, err := client.ListBuckets(context.Background(), bucketsRequest)
	if err != nil {
		usage.Error = err.Error()
		return usage
	}

	var totalBytes int64
	usage.Buckets = []BucketInfo{}

	for _, bucket := range bucketsResponse.Items {
		// Obtener detalles del bucket (incluyendo tamaño aproximado)
		bucketRequest := objectstorage.GetBucketRequest{
			NamespaceName: common.String(namespace),
			BucketName:    bucket.Name,
			Fields:        []objectstorage.GetBucketFieldsEnum{objectstorage.GetBucketFieldsApproximatesize},
		}
		bucketResponse, err := client.GetBucket(context.Background(), bucketRequest)
		if err != nil {
			usage.Buckets = append(usage.Buckets, BucketInfo{
				Name:   *bucket.Name,
				SizeGB: -1, // Indicar error
			})
			continue
		}

		var sizeGB float64
		if bucketResponse.ApproximateSize != nil {
			sizeBytes := *bucketResponse.ApproximateSize
			sizeGB = float64(sizeBytes) / (1024 * 1024 * 1024)
			totalBytes += sizeBytes
		}

		usage.Buckets = append(usage.Buckets, BucketInfo{
			Name:   *bucket.Name,
			SizeGB: sizeGB,
		})
	}

	totalGB := float64(totalBytes) / (1024 * 1024 * 1024)
	usage.Total = UsageMetric{
		Used:       totalGB,
		Limit:      float64(Limits.ObjectStorage.TotalGB),
		Percentage: int((totalGB / float64(Limits.ObjectStorage.TotalGB)) * 100),
	}

	return usage
}

// getLoadBalancerUsage obtiene el uso de load balancers
func getLoadBalancerUsage(provider common.ConfigurationProvider, compartmentID string) LoadBalancerUsage {
	usage := LoadBalancerUsage{}

	client, err := loadbalancer.NewLoadBalancerClientWithConfigurationProvider(provider)
	if err != nil {
		usage.Error = err.Error()
		return usage
	}

	request := loadbalancer.ListLoadBalancersRequest{
		CompartmentId: common.String(compartmentID),
	}

	response, err := client.ListLoadBalancers(context.Background(), request)
	if err != nil {
		usage.Error = err.Error()
		return usage
	}

	count := len(response.Items)
	usage.Count = UsageMetric{
		Used:       float64(count),
		Limit:      float64(Limits.LoadBalancer.Instances),
		Percentage: int((float64(count) / float64(Limits.LoadBalancer.Instances)) * 100),
	}

	usage.LoadBalancers = []LoadBalancerInfo{}
	for _, lb := range response.Items {
		usage.LoadBalancers = append(usage.LoadBalancers, LoadBalancerInfo{
			Name:  *lb.DisplayName,
			Shape: *lb.ShapeName,
			State: string(lb.LifecycleState),
		})
	}

	return usage
}
