package api

import (
	"testing"
	"time"

	"github.com/RadW2020/oracle-free-tier-arm-watcher/internal/source"
)

func TestDropIntervalsLabelsTheWholeStretch(t *testing.T) {
	at := func(m int) time.Time { return time.Date(2026, 9, 17, 15, m, 0, 0, time.UTC) }
	points := []source.Point{
		{T: at(14), V: 0}, {T: at(15), V: 100}, {T: at(16), V: 300}, {T: at(17), V: 0},
		{T: at(20), V: 50},
	}
	got := dropIntervals(points, time.Minute)
	if len(got) != 2 {
		t.Fatalf("intervals = %+v; want 2 separate stretches", got)
	}
	// El primer bucket con descartes termina a las 15:15, así que empieza
	// a las 15:14: el intervalo se da desde el inicio de su primer bucket.
	if !got[0].Start.Equal(at(14)) || !got[0].End.Equal(at(16)) || got[0].TotalDrops != 400 || got[0].PeakPerBucket != 300 {
		t.Errorf("first = %+v", got[0])
	}
	if !got[1].Start.Equal(at(19)) || got[1].TotalDrops != 50 {
		t.Errorf("second = %+v", got[1])
	}
}

func TestSummarizeSumsOnlySumMetrics(t *testing.T) {
	pts := []source.Point{{T: time.Unix(60, 0), V: 1}, {T: time.Unix(120, 0), V: 3}}
	drops, _ := source.MetricByName(source.MetricIngressDrops)
	cpu, _ := source.MetricByName(source.MetricCPU)
	if s := summarize(drops, pts); s.Total == nil || *s.Total != 4 || s.Max != 3 || s.Mean != 2 {
		t.Errorf("drops summary = %+v", s)
	}
	if s := summarize(cpu, pts); s.Total != nil {
		t.Error("a total of percentages means nothing")
	}
	if s := summarize(cpu, nil); s.Points != 0 || s.MaxAt != nil {
		t.Errorf("empty summary = %+v", s)
	}
}

func TestSchemasExposeEnums(t *testing.T) {
	for _, op := range Operations {
		if _, err := SchemaFor(op.Input); err != nil {
			t.Errorf("%s input: %v", op.Name, err)
		}
		if _, err := SchemaFor(op.Output); err != nil {
			t.Errorf("%s output: %v", op.Name, err)
		}
	}
	s, _ := SchemaFor(Operations[1].Input)
	if q := s.Properties["quota"]; q == nil || len(q.Enum) != 10 {
		t.Errorf("quota must be an enum of the 10 quota IDs: %+v", q)
	}
}
