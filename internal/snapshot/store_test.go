package snapshot

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/RadW2020/oracle-free-tier-arm-watcher/internal/freetier"
	"github.com/RadW2020/oracle-free-tier-arm-watcher/internal/source"
)

// countingSource cuenta lecturas y puede bloquearlas para simular una
// lectura lenta de OCI.
type countingSource struct {
	source.Source
	reads   atomic.Int32
	release chan struct{}
	err     error
}

func (c *countingSource) Read(ctx context.Context) (freetier.Reading, error) {
	c.reads.Add(1)
	if c.release != nil {
		<-c.release
	}
	if c.err != nil {
		return freetier.Reading{}, c.err
	}
	return freetier.NewReading(freetier.AllUsage{}, time.Now(), nil), nil
}

func TestCooldownProtectsOCI(t *testing.T) {
	src := &countingSource{}
	now := time.Date(2026, 9, 25, 9, 0, 0, 0, time.UTC)
	store := New(src, Options{Cooldown: time.Minute, Now: func() time.Time { return now }})

	if r := store.Refresh(context.Background(), false); !r.Refreshed || r.Reason != ReasonRefreshed {
		t.Fatalf("first refresh = %+v", r)
	}
	now = now.Add(20 * time.Second)
	r := store.Refresh(context.Background(), false)
	if r.Refreshed || r.Reason != ReasonCooldown || r.RetryAfter != 40*time.Second {
		t.Errorf("second refresh = %+v; want cooldown with 40s left", r)
	}
	if r.Snapshot == nil {
		t.Error("a cooldown must still hand back the current snapshot")
	}
	if n := src.reads.Load(); n != 1 {
		t.Errorf("reads = %d; want 1", n)
	}

	// El worker fuerza la lectura: su cadencia no depende del cooldown.
	if r := store.Refresh(context.Background(), true); !r.Refreshed {
		t.Errorf("forced refresh = %+v", r)
	}
}

func TestConcurrentRefreshesCoalesce(t *testing.T) {
	src := &countingSource{release: make(chan struct{})}
	store := New(src, Options{})

	var wg sync.WaitGroup
	results := make([]RefreshResult, 5)
	for i := range results {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = store.Refresh(context.Background(), false)
		}()
	}
	// Dar tiempo a que todas lleguen antes de soltar la lectura.
	for store.Health().Refreshing == false {
		time.Sleep(time.Millisecond)
	}
	time.Sleep(20 * time.Millisecond)
	close(src.release)
	wg.Wait()

	if n := src.reads.Load(); n != 1 {
		t.Errorf("reads = %d; want 1 for 5 concurrent callers", n)
	}
	for _, r := range results {
		if !r.Refreshed || r.Snapshot == nil {
			t.Errorf("result = %+v; every caller should get the fresh snapshot", r)
		}
	}
}

func TestFailedReadKeepsLastGoodSnapshot(t *testing.T) {
	src := &countingSource{}
	now := time.Now()
	store := New(src, Options{Now: func() time.Time { return now }})
	store.Refresh(context.Background(), true)

	src.err = source.ErrNotConfigured
	now = now.Add(time.Hour)
	r := store.Refresh(context.Background(), true)
	if r.Refreshed || r.Err == nil {
		t.Fatalf("refresh = %+v; want the error", r)
	}
	if _, ok := store.Current(); !ok {
		t.Error("a failed read must not drop the last good snapshot")
	}
	h := store.Health()
	if h.LastError == nil || !h.LastSuccessAt.Before(h.LastAttemptAt) {
		t.Errorf("health = %+v", h)
	}
}
