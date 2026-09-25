// Este archivo contiene la lógica para conectar con OCI.
package source

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/RadW2020/oracle-free-tier-arm-watcher/internal/freetier"
	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/oracle/oci-go-sdk/v65/database"
	"github.com/oracle/oci-go-sdk/v65/loadbalancer"
	"github.com/oracle/oci-go-sdk/v65/monitoring"
	"github.com/oracle/oci-go-sdk/v65/objectstorage"
)

// OCIConfig son las credenciales y el alcance de la tenancy.
type OCIConfig struct {
	TenancyID      string
	UserID         string
	Fingerprint    string
	PrivateKeyPath string
	Region         string
	CompartmentID  string
	// CallTimeout acota cada llamada al SDK. Antes todas usaban
	// context.Background(): una llamada colgada dejaba colgado al cliente.
	CallTimeout time.Duration
}

// OCIConfigFromEnv lee la configuración de las variables de entorno.
func OCIConfigFromEnv() OCIConfig {
	return OCIConfig{
		TenancyID:      os.Getenv("OCI_TENANCY_ID"),
		UserID:         os.Getenv("OCI_USER_ID"),
		Fingerprint:    os.Getenv("OCI_FINGERPRINT"),
		PrivateKeyPath: os.Getenv("OCI_PRIVATE_KEY_PATH"),
		Region:         os.Getenv("OCI_REGION"),
		CompartmentID:  os.Getenv("OCI_COMPARTMENT_ID"),
		CallTimeout:    10 * time.Second,
	}
}

// OCI es la fuente real: lee la tenancy con el SDK, sólo con operaciones
// List*, Get* y SummarizeMetricsData. Nada aquí modifica la cuenta, y así
// debe seguir: es lo que permite que el usuario de OCI del watcher tenga una
// política de sólo lectura.
type OCI struct {
	cfg OCIConfig
}

// NewOCI crea la fuente de OCI.
func NewOCI(cfg OCIConfig) *OCI {
	if cfg.CallTimeout == 0 {
		cfg.CallTimeout = 10 * time.Second
	}
	return &OCI{cfg: cfg}
}

func (o *OCI) Kind() string { return "oci" }

// Configured verifica si las credenciales de OCI están configuradas.
func (o *OCI) Configured() bool {
	c := o.cfg
	return c.TenancyID != "" && c.UserID != "" && c.Fingerprint != "" && c.PrivateKeyPath != "" && c.Region != ""
}

// MissingConfig lista las variables que faltan.
func (o *OCI) MissingConfig() []string {
	var missing []string
	for key, value := range map[string]string{
		"OCI_TENANCY_ID":       o.cfg.TenancyID,
		"OCI_USER_ID":          o.cfg.UserID,
		"OCI_FINGERPRINT":      o.cfg.Fingerprint,
		"OCI_PRIVATE_KEY_PATH": o.cfg.PrivateKeyPath,
		"OCI_REGION":           o.cfg.Region,
	} {
		if value == "" {
			missing = append(missing, key)
		}
	}
	sort.Strings(missing)
	return missing
}

// Scope describe lo que se mide. Los límites del Free Tier son de toda la
// tenancy, pero aquí sólo se lee un compartimento y NO sus hijos: un recurso
// en un compartimento hijo no cuenta. Se publica para que nadie lo descubra
// comparando números con la consola.
func (o *OCI) Scope() string {
	return fmt.Sprintf("compartment %s only (child compartments are not included)", o.compartmentID())
}

func (o *OCI) Now() time.Time { return time.Now().UTC() }

// compartmentID obtiene el ID del compartimento a monitorear.
func (o *OCI) compartmentID() string {
	if o.cfg.CompartmentID == "" {
		// Si no hay compartimento específico, usar el tenancy (root)
		return o.cfg.TenancyID
	}
	return o.cfg.CompartmentID
}

// provider crea el proveedor de autenticación de OCI. Lee la clave en cada
// lectura para que una rotación no exija reiniciar.
func (o *OCI) provider() (common.ConfigurationProvider, error) {
	privateKeyBytes, err := os.ReadFile(o.cfg.PrivateKeyPath)
	if err != nil {
		return nil, &Error{Code: "credentials_unreadable", Message: "OCI private key file cannot be read", Err: err}
	}
	return common.NewRawConfigurationProvider(
		o.cfg.TenancyID,
		o.cfg.UserID,
		o.cfg.Region,
		o.cfg.Fingerprint,
		string(privateKeyBytes),
		nil, // passphrase (nil si no tiene)
	), nil
}

func (o *OCI) call(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, o.cfg.CallTimeout)
}

// Read obtiene todo el uso de OCI de forma paralela.
func (o *OCI) Read(ctx context.Context) (freetier.Reading, error) {
	if !o.Configured() {
		return freetier.Reading{}, ErrNotConfigured
	}
	provider, err := o.provider()
	if err != nil {
		return freetier.Reading{}, err
	}
	compartmentID := o.compartmentID()

	var (
		usage    freetier.AllUsage
		mu       sync.Mutex
		wg       sync.WaitGroup
		failures = map[string]freetier.SourceStatus{}
	)
	record := func(name string, err error) {
		if err == nil {
			return
		}
		mu.Lock()
		failures[name] = statusFor(name, err)
		mu.Unlock()
	}

	// Cada fuente va en su goroutine: son independientes y la latencia total
	// es la de la más lenta, no la suma.
	fetchers := map[string]func() error{
		freetier.SourceCompute: func() (err error) {
			usage.Compute, err = o.computeUsage(ctx, provider, compartmentID)
			return err
		},
		freetier.SourceBlockStorage: func() (err error) {
			usage.BlockStorage, err = o.blockStorageUsage(ctx, provider, compartmentID)
			return err
		},
		freetier.SourceObjectStorage: func() (err error) {
			usage.ObjectStorage, err = o.objectStorageUsage(ctx, provider, compartmentID)
			return err
		},
		freetier.SourceLoadBalancer: func() (err error) {
			usage.LoadBalancer, err = o.loadBalancerUsage(ctx, provider, compartmentID)
			return err
		},
		freetier.SourcePublicIPs: func() (err error) {
			usage.PublicIPs, err = o.publicIPsUsage(ctx, provider, compartmentID)
			return err
		},
		freetier.SourceDatabase: func() (err error) {
			usage.Database, err = o.databaseUsage(ctx, provider, compartmentID)
			return err
		},
		freetier.SourceBandwidth: func() (err error) {
			usage.Bandwidth, err = o.bandwidthUsage(ctx, provider, compartmentID)
			return err
		},
		freetier.SourceSaturation: func() (err error) {
			usage.Saturation, err = o.saturationUsage(ctx, provider, compartmentID)
			return err
		},
	}
	for name, fetch := range fetchers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			record(name, fetch())
		}()
	}
	wg.Wait()

	freetier.Finalize(&usage)
	return freetier.NewReading(usage, o.Now(), failures), nil
}

// publicIPsUsage monitoriza las IPs públicas reservadas (límite free tier: 2).
//
// Antes esta función se tragaba los errores y devolvía 0 de 2: un permiso
// que faltase era indistinguible de no tener IPs reservadas.
func (o *OCI) publicIPsUsage(ctx context.Context, provider common.ConfigurationProvider, compartmentID string) (freetier.UsageMetric, error) {
	usage := freetier.UsageMetric{}

	client, err := core.NewVirtualNetworkClientWithConfigurationProvider(provider)
	if err != nil {
		return usage, err
	}

	callCtx, cancel := o.call(ctx)
	defer cancel()
	response, err := client.ListPublicIps(callCtx, core.ListPublicIpsRequest{
		CompartmentId: common.String(compartmentID),
		Scope:         core.ListPublicIpsScopeRegion,
	})
	if err != nil {
		return usage, err
	}

	usage.Used = float64(len(response.Items))
	return usage, nil
}

// computeUsage obtiene el uso de compute.
func (o *OCI) computeUsage(ctx context.Context, provider common.ConfigurationProvider, compartmentID string) (freetier.ComputeUsage, error) {
	usage := freetier.ComputeUsage{}

	client, err := core.NewComputeClientWithConfigurationProvider(provider)
	if err != nil {
		usage.Error = err.Error()
		return usage, err
	}

	// Sólo instancias en ejecución: si una STOPPED cuenta o no contra el
	// límite de A1 no está verificado aquí, y la descripción de la cuota lo
	// dice para que nadie lo dé por supuesto.
	callCtx, cancel := o.call(ctx)
	defer cancel()
	response, err := client.ListInstances(callCtx, core.ListInstancesRequest{
		CompartmentId:  common.String(compartmentID),
		LifecycleState: core.InstanceLifecycleStateRunning,
	})
	if err != nil {
		usage.Error = err.Error()
		return usage, err
	}

	var armOCPUs, armMemoryGB float64
	var armCount, amdCount int
	usage.InstanceDetails = []freetier.InstanceInfo{}

	for _, instance := range response.Items {
		shape := *instance.Shape
		info := freetier.InstanceInfo{Name: deref(instance.DisplayName), Shape: shape, State: string(instance.LifecycleState)}
		if instance.ShapeConfig != nil {
			if instance.ShapeConfig.Ocpus != nil {
				info.OCPUs = float64(*instance.ShapeConfig.Ocpus)
			}
			if instance.ShapeConfig.MemoryInGBs != nil {
				info.MemoryGB = float64(*instance.ShapeConfig.MemoryInGBs)
			}
		}
		usage.InstanceDetails = append(usage.InstanceDetails, info)

		// Detectar si es ARM (Ampere) o AMD
		if strings.Contains(shape, "A1") || strings.Contains(shape, "Ampere") {
			armOCPUs += info.OCPUs
			armMemoryGB += info.MemoryGB
			armCount++
		} else if strings.Contains(shape, "Micro") {
			amdCount++
		}
	}

	usage.ARM.OCPUs.Used = armOCPUs
	usage.ARM.MemoryGB.Used = armMemoryGB
	usage.ARM.Instances = armCount
	usage.AMD.Instances.Used = float64(amdCount)
	usage.TotalInstances = len(response.Items)
	return usage, nil
}

// blockStorageUsage obtiene el uso de block storage.
func (o *OCI) blockStorageUsage(ctx context.Context, provider common.ConfigurationProvider, compartmentID string) (freetier.StorageUsage, error) {
	usage := freetier.StorageUsage{Volumes: []freetier.VolumeInfo{}}

	client, err := core.NewBlockstorageClientWithConfigurationProvider(provider)
	if err != nil {
		usage.Error = err.Error()
		return usage, err
	}

	callCtx, cancel := o.call(ctx)
	defer cancel()
	bootResponse, err := client.ListBootVolumes(callCtx, core.ListBootVolumesRequest{
		CompartmentId: common.String(compartmentID),
	})
	if err != nil {
		usage.Error = err.Error()
		return usage, err
	}

	var bootVolumeGB int64
	for _, vol := range bootResponse.Items {
		size := derefInt64(vol.SizeInGBs)
		bootVolumeGB += size
		usage.Volumes = append(usage.Volumes, freetier.VolumeInfo{
			Name: deref(vol.DisplayName), Kind: "boot", SizeGB: int(size), State: string(vol.LifecycleState),
		})
	}
	usage.BootVolumes.Count = len(bootResponse.Items)
	usage.BootVolumes.SizeGB = int(bootVolumeGB)

	callCtx2, cancel2 := o.call(ctx)
	defer cancel2()
	blockResponse, err := client.ListVolumes(callCtx2, core.ListVolumesRequest{
		CompartmentId: common.String(compartmentID),
	})
	if err != nil {
		usage.Error = err.Error()
		return usage, err
	}

	var blockVolumeGB int64
	for _, vol := range blockResponse.Items {
		size := derefInt64(vol.SizeInGBs)
		blockVolumeGB += size
		usage.Volumes = append(usage.Volumes, freetier.VolumeInfo{
			Name: deref(vol.DisplayName), Kind: "block", SizeGB: int(size), State: string(vol.LifecycleState),
		})
	}
	usage.BlockVolumes.Count = len(blockResponse.Items)
	usage.BlockVolumes.SizeGB = int(blockVolumeGB)

	usage.Total.Used = float64(bootVolumeGB + blockVolumeGB)
	return usage, nil
}

// objectStorageUsage obtiene el uso de object storage.
func (o *OCI) objectStorageUsage(ctx context.Context, provider common.ConfigurationProvider, compartmentID string) (freetier.ObjectStorageUsage, error) {
	usage := freetier.ObjectStorageUsage{Buckets: []freetier.BucketInfo{}}

	client, err := objectstorage.NewObjectStorageClientWithConfigurationProvider(provider)
	if err != nil {
		usage.Error = err.Error()
		return usage, err
	}

	// Obtener namespace (requerido para object storage)
	callCtx, cancel := o.call(ctx)
	defer cancel()
	nsResponse, err := client.GetNamespace(callCtx, objectstorage.GetNamespaceRequest{})
	if err != nil {
		usage.Error = err.Error()
		return usage, err
	}
	namespace := *nsResponse.Value

	callCtx2, cancel2 := o.call(ctx)
	defer cancel2()
	bucketsResponse, err := client.ListBuckets(callCtx2, objectstorage.ListBucketsRequest{
		NamespaceName: common.String(namespace),
		CompartmentId: common.String(compartmentID),
	})
	if err != nil {
		usage.Error = err.Error()
		return usage, err
	}

	var totalBytes int64
	for _, bucket := range bucketsResponse.Items {
		// Obtener detalles del bucket (incluyendo tamaño aproximado)
		bucketCtx, bucketCancel := o.call(ctx)
		bucketResponse, err := client.GetBucket(bucketCtx, objectstorage.GetBucketRequest{
			NamespaceName: common.String(namespace),
			BucketName:    bucket.Name,
			Fields:        []objectstorage.GetBucketFieldsEnum{objectstorage.GetBucketFieldsApproximatesize},
		})
		bucketCancel()
		if err != nil {
			// -1 es el centinela heredado; SizeKnown lo dice sin trampas.
			usage.Buckets = append(usage.Buckets, freetier.BucketInfo{Name: *bucket.Name, SizeGB: -1})
			continue
		}

		var sizeGB float64
		if bucketResponse.ApproximateSize != nil {
			sizeBytes := *bucketResponse.ApproximateSize
			sizeGB = float64(sizeBytes) / (1024 * 1024 * 1024)
			totalBytes += sizeBytes
		}
		usage.Buckets = append(usage.Buckets, freetier.BucketInfo{Name: *bucket.Name, SizeGB: sizeGB, SizeKnown: true})
	}

	usage.Total.Used = float64(totalBytes) / (1024 * 1024 * 1024)
	return usage, nil
}

// loadBalancerUsage obtiene el uso de load balancers.
func (o *OCI) loadBalancerUsage(ctx context.Context, provider common.ConfigurationProvider, compartmentID string) (freetier.LoadBalancerUsage, error) {
	usage := freetier.LoadBalancerUsage{LoadBalancers: []freetier.LoadBalancerInfo{}}

	client, err := loadbalancer.NewLoadBalancerClientWithConfigurationProvider(provider)
	if err != nil {
		usage.Error = err.Error()
		return usage, err
	}

	callCtx, cancel := o.call(ctx)
	defer cancel()
	response, err := client.ListLoadBalancers(callCtx, loadbalancer.ListLoadBalancersRequest{
		CompartmentId: common.String(compartmentID),
	})
	if err != nil {
		usage.Error = err.Error()
		return usage, err
	}

	usage.Count.Used = float64(len(response.Items))
	for _, lb := range response.Items {
		usage.LoadBalancers = append(usage.LoadBalancers, freetier.LoadBalancerInfo{
			Name:  deref(lb.DisplayName),
			Shape: deref(lb.ShapeName),
			State: string(lb.LifecycleState),
		})
	}
	return usage, nil
}

// databaseUsage obtiene el uso de Autonomous Databases.
func (o *OCI) databaseUsage(ctx context.Context, provider common.ConfigurationProvider, compartmentID string) (freetier.DatabaseUsage, error) {
	usage := freetier.DatabaseUsage{}

	client, err := database.NewDatabaseClientWithConfigurationProvider(provider)
	if err != nil {
		usage.Error = err.Error()
		return usage, err
	}

	callCtx, cancel := o.call(ctx)
	defer cancel()
	response, err := client.ListAutonomousDatabases(callCtx, database.ListAutonomousDatabasesRequest{
		CompartmentId: common.String(compartmentID),
	})
	if err != nil {
		usage.Error = err.Error()
		return usage, err
	}

	count := 0
	var storageTB float64
	for _, db := range response.Items {
		// Solo contar si no está eliminado
		if db.LifecycleState != database.AutonomousDatabaseSummaryLifecycleStateTerminated {
			count++
			if db.DataStorageSizeInTBs != nil {
				storageTB += float64(*db.DataStorageSizeInTBs)
			}
		}
	}

	usage.Count = count
	usage.AutonomousDBs.Used = float64(count)
	usage.StorageUsage.Used = storageTB * 1024
	return usage, nil
}

// bandwidthUsage obtiene el uso de transferencia (egress) del mes actual
// usando la API de Monitoring de OCI (VnicToNetworkBytes).
func (o *OCI) bandwidthUsage(ctx context.Context, provider common.ConfigurationProvider, compartmentID string) (freetier.BandwidthUsage, error) {
	usage := freetier.BandwidthUsage{}

	client, err := monitoring.NewMonitoringClientWithConfigurationProvider(provider)
	if err != nil {
		usage.Error = err.Error()
		return usage, err
	}

	now := o.Now()
	startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)

	callCtx, cancel := o.call(ctx)
	defer cancel()
	response, err := client.SummarizeMetricsData(callCtx, monitoring.SummarizeMetricsDataRequest{
		CompartmentId: common.String(compartmentID),
		SummarizeMetricsDataDetails: monitoring.SummarizeMetricsDataDetails{
			Namespace: common.String("oci_vcn"),
			Query:     common.String("VnicToNetworkBytes[1d].sum()"),
			StartTime: &common.SDKTime{Time: startOfMonth},
			EndTime:   &common.SDKTime{Time: now},
		},
	})
	if err != nil {
		usage.Error = err.Error()
		return usage, err
	}

	var totalBytes float64
	for _, metric := range response.Items {
		for _, datapoint := range metric.AggregatedDatapoints {
			if datapoint.Value != nil {
				totalBytes += *datapoint.Value
			}
		}
	}

	usage.EgressGB = totalBytes / (1024 * 1024 * 1024)
	return usage, nil
}

// ---------------------------------------------------------------------------
// Señales de saturación (no son cuota)
// ---------------------------------------------------------------------------

// Series devuelve una serie de OCI Monitoring para una ventana.
func (o *OCI) Series(ctx context.Context, q SeriesQuery) (Series, error) {
	spec, ok := MetricByName(q.Metric)
	if !ok {
		return Series{}, &Error{Code: "invalid_metric", Message: "unknown metric " + string(q.Metric)}
	}
	if !o.Configured() {
		return Series{}, ErrNotConfigured
	}
	provider, err := o.provider()
	if err != nil {
		return Series{}, err
	}
	client, err := monitoring.NewMonitoringClientWithConfigurationProvider(provider)
	if err != nil {
		return Series{}, err
	}
	query := spec.Query(q.Resolution)
	points, err := o.queryMonitoringSeries(ctx, client, o.compartmentID(), spec, query, q.Start, q.End)
	if err != nil {
		return Series{}, err
	}
	return Series{Spec: spec, Query: query, Points: points}, nil
}

// queryMonitoringSeries ejecuta una query MQL y devuelve los datapoints
// ordenados de más antiguo a más reciente.
//
// Si hay varias series (varias VNICs), se combinan por instante: se suman
// los bytes y los paquetes y se promedian los porcentajes. Antes se tomaba
// sólo la primera serie, que con una VNIC da lo mismo y con dos mentiría.
//
// La API de Monitoring etiqueta cada bucket con el FINAL de su intervalo: en
// una serie [1m], el punto de las 15:15 contiene 15:14:00-15:15:00.
func (o *OCI) queryMonitoringSeries(ctx context.Context, client monitoring.MonitoringClient, compartmentID string, spec MetricSpec, query string, start, end time.Time) ([]Point, error) {
	callCtx, cancel := o.call(ctx)
	defer cancel()
	response, err := client.SummarizeMetricsData(callCtx, monitoring.SummarizeMetricsDataRequest{
		CompartmentId: common.String(compartmentID),
		SummarizeMetricsDataDetails: monitoring.SummarizeMetricsDataDetails{
			Namespace: common.String(spec.Namespace),
			Query:     common.String(query),
			StartTime: &common.SDKTime{Time: start},
			EndTime:   &common.SDKTime{Time: end},
		},
	})
	if err != nil {
		return nil, err
	}

	sums := map[time.Time]float64{}
	counts := map[time.Time]int{}
	for _, item := range response.Items {
		for _, dp := range item.AggregatedDatapoints {
			if dp.Value == nil || dp.Timestamp == nil {
				continue
			}
			t := dp.Timestamp.Time.UTC()
			sums[t] += *dp.Value
			counts[t]++
		}
	}
	points := make([]Point, 0, len(sums))
	for t, v := range sums {
		if spec.Aggregation == "mean" {
			v /= float64(counts[t])
		}
		points = append(points, Point{T: t, V: v})
	}
	sort.Slice(points, func(i, j int) bool { return points[i].T.Before(points[j].T) })
	return points, nil
}

// saturationUsage obtiene las señales que explican una degradación del
// host: CPU, memoria, tráfico real de la VNIC y paquetes descartados por el
// shaper de OCI.
func (o *OCI) saturationUsage(ctx context.Context, provider common.ConfigurationProvider, compartmentID string) (freetier.SaturationUsage, error) {
	client, err := monitoring.NewMonitoringClientWithConfigurationProvider(provider)
	if err != nil {
		return freetier.SaturationUsage{Error: err.Error()}, err
	}
	now := o.Now()
	return SaturationFromSeries(now, func(m Metric, window time.Duration) ([]Point, error) {
		spec, _ := MetricByName(m)
		return o.queryMonitoringSeries(ctx, client, compartmentID, spec, spec.Query(time.Minute), now.Add(-window), now)
	})
}

// SaturationFromSeries calcula los valores instantáneos de saturación a
// partir de series de 1 minuto. La comparten la fuente de OCI y la de
// fixtures, para que el mismo escenario no pueda dar un número en /usage y
// otro en la línea de tiempo.
func SaturationFromSeries(now time.Time, fetch func(Metric, time.Duration) ([]Point, error)) (freetier.SaturationUsage, error) {
	usage := freetier.SaturationUsage{}

	// Ventana corta para los valores instantáneos: suficiente para cubrir el
	// retraso de publicación de Monitoring sin traerse media hora de puntos.
	const shortWindow = 20 * time.Minute
	maxAge := time.Duration(0)
	var errs []error

	latest := func(m Metric, window time.Duration, set func(v float64), trackAge bool) []Point {
		points, err := fetch(m, window)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", m, err))
			return nil
		}
		if value, t, ok := latestPoint(points); ok {
			set(value)
			if age := now.Sub(t); trackAge && age > maxAge {
				maxAge = age
			}
		} else {
			usage.MissingSignals = append(usage.MissingSignals, string(m))
		}
		return points
	}

	latest(MetricCPU, shortWindow, func(v float64) { usage.CPUPercentage = v }, true)
	latest(MetricMemory, shortWindow, func(v float64) { usage.MemoryPercentage = v }, false)
	latest(MetricIngressBytes, shortWindow, func(v float64) { usage.IngressMBPerMin = v / (1024 * 1024) }, true)
	latest(MetricEgressBytes, shortWindow, func(v float64) { usage.EgressMBPerMin = v / (1024 * 1024) }, false)

	// Los descartes se piden a una hora y no al minuto: son ráfagas cortas
	// (el 17/09 duraron seis minutos) y el worker sólo refresca cada 15 min,
	// así que preguntar sólo por el último minuto los perdería casi siempre.
	drops := latest(MetricIngressDrops, time.Hour, func(v float64) { usage.IngressDropsPerMin = v }, false)
	for _, p := range drops {
		usage.IngressDropsLastHour += p.V
	}

	usage.SampleAgeSeconds = int(maxAge.Seconds())
	if len(errs) > 0 {
		err := errors.Join(errs...)
		usage.Error = err.Error()
		return usage, err
	}
	return usage, nil
}

// latestPoint devuelve el valor y el instante del punto más reciente.
//
// Hace falta el instante porque Monitoring publica con unos minutos de
// retraso: dar el valor sin decir de cuándo es invita a leer un pico viejo
// como si estuviera pasando ahora.
func latestPoint(points []Point) (float64, time.Time, bool) {
	if len(points) == 0 {
		return 0, time.Time{}, false
	}
	p := points[len(points)-1]
	return p.V, p.T, true
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func derefInt64(v *int64) int64 {
	if v == nil {
		return 0
	}
	return *v
}
