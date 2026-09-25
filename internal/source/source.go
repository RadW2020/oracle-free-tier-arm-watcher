// Package source abstrae de dónde salen los datos: la tenancy real de OCI o
// un escenario de fixtures.
//
// Antes cada función de oci.go creaba su propio cliente del SDK, así que nada
// por debajo de assessQuotas se podía probar sin credenciales reales, y el
// proyecto no se podía enseñar sin una tenancy. Con esta interfaz, los tests,
// las evals y el demo de `docker compose up` corren contra escenarios
// deterministas, y la fuente de OCI es una implementación más.
package source

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/RadW2020/oracle-free-tier-arm-watcher/internal/freetier"
	"github.com/oracle/oci-go-sdk/v65/common"
)

// Source es una fuente de datos de la tenancy.
type Source interface {
	// Kind es "oci" o "fixture". Se publica en cada respuesta de la API /v1
	// para que nadie confunda un escenario con la cuenta real.
	Kind() string
	Configured() bool
	// Scope describe qué parte de la tenancy se mide.
	Scope() string
	// Read toma una lectura completa. Sólo devuelve error si no se pudo
	// medir nada (sin credenciales, clave ilegible); los fallos de una fuente
	// concreta van en Reading.Sources.
	Read(ctx context.Context) (freetier.Reading, error)
	// Series devuelve una serie temporal de OCI Monitoring.
	Series(ctx context.Context, q SeriesQuery) (Series, error)
	// Now es el reloj de los datos. En los fixtures está congelado, que es
	// lo que hace reproducibles las evals.
	Now() time.Time
}

// Metric es una señal de saturación consultable en el tiempo.
type Metric string

const (
	MetricIngressBytes Metric = "ingress_bytes"
	MetricEgressBytes  Metric = "egress_bytes"
	MetricIngressDrops Metric = "ingress_throttle_drops"
	MetricCPU          Metric = "cpu_percent"
	MetricMemory       Metric = "memory_percent"
)

// MetricSpec traduce una Metric a su consulta de OCI Monitoring.
type MetricSpec struct {
	Metric      Metric
	Namespace   string
	Name        string
	Aggregation string // "sum" | "mean"
	Unit        string
	Description string
}

// Query compone la MQL para una resolución.
func (m MetricSpec) Query(resolution time.Duration) string {
	return fmt.Sprintf("%s[%s].%s()", m.Name, mqlInterval(resolution), m.Aggregation)
}

func mqlInterval(d time.Duration) string {
	if d >= time.Hour && d%time.Hour == 0 {
		return fmt.Sprintf("%dh", int(d/time.Hour))
	}
	return fmt.Sprintf("%dm", int(d/time.Minute))
}

// Metrics es el catálogo de señales. El tráfico sale de oci_vcn y no de
// oci_computeagent a propósito: NetworksBytesIn/Out del agente suman TODAS
// las interfaces, incluidas las de Docker, y en esta máquina marcan
// ~50 GB/día de salida cuando lo que sale de verdad a internet son 8,5 GB/día.
var Metrics = []MetricSpec{
	{MetricIngressBytes, "oci_vcn", "VnicFromNetworkBytes", "sum", "bytes", "Inbound bytes on the instance VNIC (real internet traffic, excludes Docker interfaces)."},
	{MetricEgressBytes, "oci_vcn", "VnicToNetworkBytes", "sum", "bytes", "Outbound bytes on the instance VNIC."},
	{MetricIngressDrops, "oci_vcn", "VnicIngressDropsThrottle", "sum", "packets", "Inbound packets dropped by the OCI VNIC shaper because ingress exceeded the shape's bandwidth. Any value > 0 means other services on the host are losing packets, including SYNs."},
	{MetricCPU, "oci_computeagent", "CpuUtilization", "mean", "percent", "CPU utilization of the instance."},
	{MetricMemory, "oci_computeagent", "MemoryUtilization", "mean", "percent", "Memory utilization of the instance."},
}

// MetricByName busca una señal del catálogo.
func MetricByName(m Metric) (MetricSpec, bool) {
	for _, spec := range Metrics {
		if spec.Metric == m {
			return spec, true
		}
	}
	return MetricSpec{}, false
}

// SeriesQuery pide una serie para una ventana y una resolución.
type SeriesQuery struct {
	Metric     Metric
	Start, End time.Time
	Resolution time.Duration
}

// Point es un punto de una serie. OCI Monitoring etiqueta cada bucket con el
// FINAL de su intervalo: en una serie [1m], el punto de las 15:15 contiene
// 15:14:00-15:15:00.
type Point struct {
	T time.Time `json:"t"`
	V float64   `json:"v"`
}

// Series es una serie temporal con su procedencia.
type Series struct {
	Spec   MetricSpec
	Query  string
	Points []Point
}

// Error es un fallo de una fuente con un código estable.
type Error struct {
	Code    string
	Message string
	Err     error
}

func (e *Error) Error() string {
	if e.Err != nil {
		return e.Message + ": " + e.Err.Error()
	}
	return e.Message
}

func (e *Error) Unwrap() error { return e.Err }

// ErrNotConfigured indica que faltan las credenciales de OCI.
var ErrNotConfigured = &Error{Code: "not_configured", Message: "OCI credentials not configured"}

// Classify convierte un error en un código estable y un mensaje apto para
// clientes.
//
// El mensaje crudo del SDK puede llevar OCIDs e IDs de petición: va a los
// logs, no a quien llama. Al cliente le basta con saber qué tipo de fallo es
// y si reintentar tiene sentido.
func Classify(err error) (code, message string) {
	if err == nil {
		return "", ""
	}
	var srcErr *Error
	if errors.As(err, &srcErr) {
		return srcErr.Code, srcErr.Message
	}
	if se, ok := common.IsServiceError(err); ok {
		status := se.GetHTTPStatusCode()
		message = fmt.Sprintf("OCI returned HTTP %d %s", status, se.GetCode())
		switch {
		case status == 401:
			return "oci_auth", message
		case status == 403 || (status == 404 && se.GetCode() == "NotAuthorizedOrNotFound"):
			return "oci_permission", message
		case status == 429:
			return "oci_throttled", message
		case status >= 500:
			return "oci_unavailable", message
		}
		return "oci_error", message
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout", "OCI call timed out"
	}
	if errors.Is(err, context.Canceled) {
		return "cancelled", "request cancelled"
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return "network", "network error reaching OCI"
	}
	return "internal", "unexpected error reading OCI"
}

// Retryable dice si un código de fallo puede resolverse reintentando.
func Retryable(code string) bool {
	switch code {
	case "oci_throttled", "oci_unavailable", "timeout", "network":
		return true
	}
	return false
}

// statusFor construye el estado de una fuente a partir de su error.
func statusFor(name string, err error) freetier.SourceStatus {
	if err == nil {
		return freetier.SourceStatus{Name: name, Available: true}
	}
	code, message := Classify(err)
	return freetier.SourceStatus{Name: name, Available: false, ErrorCode: code, Message: message}
}
