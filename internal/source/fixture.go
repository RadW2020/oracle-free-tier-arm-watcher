package source

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"math"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/RadW2020/oracle-free-tier-arm-watcher/internal/freetier"
)

//go:embed scenarios/*.json
var scenarioFiles embed.FS

// Scenario es un estado de la tenancy reproducible: el uso, las fuentes que
// fallan y cómo son las series de Monitoring alrededor de un instante fijo.
//
// Las series no se guardan punto a punto: se generan a partir de una línea
// base y unos eventos, de forma determinista. Así un escenario de cuatro días
// a resolución de minuto cabe en unas líneas de JSON y cualquiera puede leer
// qué pasa en él.
type Scenario struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	// Synthetic dice que los datos no son una exportación real. Todos los
	// escenarios de este repo lo son; se publica para que nadie cite un
	// número del demo como si fuera de la cuenta.
	Synthetic          bool                             `json:"synthetic"`
	Now                time.Time                        `json:"now"`
	Configured         *bool                            `json:"configured,omitempty"`
	SnapshotAgeSeconds int                              `json:"snapshotAgeSeconds,omitempty"`
	Compartment        string                           `json:"compartment,omitempty"`
	Usage              freetier.AllUsage                `json:"usage"`
	Failures           map[string]freetier.SourceStatus `json:"failures,omitempty"`
	Series             SeriesSpec                       `json:"series"`
}

// SeriesSpec describe las series de Monitoring de un escenario.
type SeriesSpec struct {
	From       time.Time          `json:"from"`
	LagSeconds int                `json:"lagSeconds"`
	Baseline   map[Metric]float64 `json:"baseline"`
	// Jitter es la variación relativa (0,1 = ±10 %) sobre la línea base,
	// para que las series no sean planas y un agente tenga que leerlas.
	Jitter map[Metric]float64 `json:"jitter,omitempty"`
	Events []Event            `json:"events,omitempty"`
}

// Event fija el valor por minuto de una métrica en (Start, End]. Con Daily
// se repite a la misma hora cada día del escenario.
type Event struct {
	Metric Metric    `json:"metric"`
	Start  time.Time `json:"start"`
	End    time.Time `json:"end"`
	Value  float64   `json:"value"`
	Daily  bool      `json:"daily,omitempty"`
	Note   string    `json:"note,omitempty"`
}

// ScenarioNames lista los escenarios incluidos en el binario.
func ScenarioNames() []string {
	entries, _ := scenarioFiles.ReadDir("scenarios")
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, strings.TrimSuffix(e.Name(), ".json"))
	}
	sort.Strings(names)
	return names
}

// LoadScenario carga un escenario por nombre.
func LoadScenario(name string) (Scenario, error) {
	raw, err := scenarioFiles.ReadFile(path.Join("scenarios", name+".json"))
	if err != nil {
		return Scenario{}, fmt.Errorf("unknown scenario %q (available: %s)", name, strings.Join(ScenarioNames(), ", "))
	}
	var s Scenario
	if err := json.Unmarshal(raw, &s); err != nil {
		return Scenario{}, fmt.Errorf("scenario %q: %w", name, err)
	}
	if s.Now.IsZero() {
		return Scenario{}, fmt.Errorf("scenario %q: missing now", name)
	}
	return s, nil
}

// Fixture es la fuente que reproduce un escenario.
type Fixture struct {
	sc Scenario
}

// NewFixture crea la fuente de un escenario.
func NewFixture(sc Scenario) *Fixture { return &Fixture{sc: sc} }

func (f *Fixture) Kind() string { return "fixture" }

func (f *Fixture) Configured() bool { return f.sc.Configured == nil || *f.sc.Configured }

func (f *Fixture) Scope() string {
	compartment := f.sc.Compartment
	if compartment == "" {
		compartment = "ocid1.tenancy.oc1..fixture"
	}
	return fmt.Sprintf("fixture scenario %q, synthetic data (compartment %s only, child compartments are not included)", f.sc.Name, compartment)
}

// Now es el instante congelado del escenario.
func (f *Fixture) Now() time.Time { return f.sc.Now.UTC() }

// Scenario devuelve el escenario que reproduce.
func (f *Fixture) Scenario() Scenario { return f.sc }

// Read devuelve el uso del escenario. Los valores de saturación no se toman
// del JSON: se calculan de las propias series con el mismo código que usa la
// fuente de OCI, para que /usage y la línea de tiempo no puedan discrepar.
func (f *Fixture) Read(ctx context.Context) (freetier.Reading, error) {
	if !f.Configured() {
		return freetier.Reading{}, ErrNotConfigured
	}
	usage := f.sc.Usage
	now := f.Now()

	if _, failed := f.sc.Failures[freetier.SourceSaturation]; !failed {
		sat, err := SaturationFromSeries(now, func(m Metric, window time.Duration) ([]Point, error) {
			return f.points(m, now.Add(-window), now, time.Minute), nil
		})
		if err != nil {
			return freetier.Reading{}, err
		}
		usage.Saturation = sat
	}

	// Una fuente que falla en OCI deja su sección a cero con el error
	// escrito; aquí se reproduce igual.
	for name, failure := range f.sc.Failures {
		msg := failure.Message
		if msg == "" {
			msg = failure.ErrorCode
		}
		switch name {
		case freetier.SourceCompute:
			usage.Compute = freetier.ComputeUsage{Error: msg}
		case freetier.SourceBlockStorage:
			usage.BlockStorage = freetier.StorageUsage{Error: msg}
		case freetier.SourceObjectStorage:
			usage.ObjectStorage = freetier.ObjectStorageUsage{Error: msg}
		case freetier.SourceLoadBalancer:
			usage.LoadBalancer = freetier.LoadBalancerUsage{Error: msg}
		case freetier.SourcePublicIPs:
			usage.PublicIPs = freetier.UsageMetric{}
		case freetier.SourceDatabase:
			usage.Database = freetier.DatabaseUsage{Error: msg}
		case freetier.SourceBandwidth:
			usage.Bandwidth = freetier.BandwidthUsage{Error: msg}
		case freetier.SourceSaturation:
			usage.Saturation = freetier.SaturationUsage{Error: msg}
		}
	}

	freetier.Finalize(&usage)
	observedAt := now.Add(-time.Duration(f.sc.SnapshotAgeSeconds) * time.Second)
	return freetier.NewReading(usage, observedAt, f.sc.Failures), nil
}

// Series genera la serie de un escenario para una ventana.
func (f *Fixture) Series(ctx context.Context, q SeriesQuery) (Series, error) {
	spec, ok := MetricByName(q.Metric)
	if !ok {
		return Series{}, &Error{Code: "invalid_metric", Message: "unknown metric " + string(q.Metric)}
	}
	if !f.Configured() {
		return Series{}, ErrNotConfigured
	}
	if failure, failed := f.sc.Failures[freetier.SourceSaturation]; failed {
		return Series{}, &Error{Code: failure.ErrorCode, Message: failure.Message}
	}
	resolution := q.Resolution
	if resolution <= 0 {
		resolution = time.Minute
	}
	return Series{Spec: spec, Query: spec.Query(resolution), Points: f.points(q.Metric, q.Start, q.End, resolution)}, nil
}

// points genera los puntos de una métrica en (start, end], agregados a la
// resolución pedida con la misma semántica que OCI: suma para bytes y
// paquetes, media para porcentajes, y cada bucket etiquetado con su final.
func (f *Fixture) points(m Metric, start, end time.Time, resolution time.Duration) []Point {
	spec, _ := MetricByName(m)
	newest := f.Now().Add(-time.Duration(f.sc.Series.LagSeconds) * time.Second).Truncate(time.Minute)
	if end.After(newest) {
		end = newest
	}
	first := start.Truncate(time.Minute).Add(time.Minute)
	if from := f.sc.Series.From; !from.IsZero() && first.Before(from.Add(time.Minute)) {
		first = from.Truncate(time.Minute).Add(time.Minute)
	}

	type bucket struct {
		sum   float64
		count int
	}
	buckets := map[time.Time]*bucket{}
	for t := first; !t.After(end); t = t.Add(time.Minute) {
		label := t
		if resolution > time.Minute {
			label = t.Add(-time.Nanosecond).Truncate(resolution).Add(resolution)
		}
		b := buckets[label]
		if b == nil {
			b = &bucket{}
			buckets[label] = b
		}
		b.sum += f.valueAt(m, t)
		b.count++
	}

	points := make([]Point, 0, len(buckets))
	for t, b := range buckets {
		v := b.sum
		if spec.Aggregation == "mean" {
			v /= float64(b.count)
		}
		points = append(points, Point{T: t, V: round(v)})
	}
	sort.Slice(points, func(i, j int) bool { return points[i].T.Before(points[j].T) })
	return points
}

// valueAt es el valor de una métrica en el minuto que termina en t.
func (f *Fixture) valueAt(m Metric, t time.Time) float64 {
	for _, e := range f.sc.Series.Events {
		if e.Metric != m {
			continue
		}
		start, end := e.Start, e.End
		if e.Daily {
			days := int(math.Floor(t.Sub(start).Hours() / 24))
			start = start.AddDate(0, 0, days)
			end = end.AddDate(0, 0, days)
		}
		if t.After(start) && !t.After(end) {
			return e.Value
		}
	}
	base := f.sc.Series.Baseline[m]
	if j := f.sc.Series.Jitter[m]; j > 0 {
		base *= 1 + j*noise(m, t)
	}
	return base
}

// noise es un pseudoaleatorio determinista en [-1, 1] para cada minuto.
func noise(m Metric, t time.Time) float64 {
	h := fnv.New64a()
	fmt.Fprintf(h, "%s|%d", m, t.Unix())
	return float64(h.Sum64()%2001)/1000 - 1
}

func round(v float64) float64 {
	if v > 1000 {
		return float64(int64(v + 0.5))
	}
	return float64(int64(v*100+0.5)) / 100
}
