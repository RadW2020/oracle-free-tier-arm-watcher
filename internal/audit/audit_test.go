package audit

import (
	"io"
	"strings"
	"testing"

	"github.com/rs/zerolog"
)

func TestParseClients(t *testing.T) {
	r, err := ParseClients("checkly:read:k1, claude:refresh+read:k2", "legacy-key")
	if err != nil {
		t.Fatal(err)
	}
	if !r.Enforced() || !r.Configured() {
		t.Fatal("a registry with clients must be enforced")
	}

	c, ok := r.Authenticate("k2")
	if !ok || c.Name != "claude" || !c.Has(ScopeRefresh) || !c.Has(ScopeRead) || c.Has(ScopeAudit) {
		t.Errorf("k2 = %+v, %v", c, ok)
	}
	if c, ok := r.Authenticate("legacy-key"); !ok || c.Name != "legacy" || c.Has(ScopeRefresh) {
		t.Errorf("legacy = %+v, %v; the old API_KEY keeps read access only", c, ok)
	}
	for _, bad := range []string{"", "k3", "k1 ", "K1"} {
		if _, ok := r.Authenticate(bad); ok {
			t.Errorf("key %q must not authenticate", bad)
		}
	}
}

func TestParseClientsRejectsBadConfig(t *testing.T) {
	for _, cfg := range []string{"nameonly", "a:read", "a:admin:k", "a:read:k,a:read:k2", ":read:k"} {
		_, err := ParseClients(cfg, "")
		if err == nil {
			t.Errorf("%q should be rejected", cfg)
			continue
		}
		if strings.Contains(err.Error(), ":k") && !strings.Contains(err.Error(), "***") {
			t.Errorf("error leaks the key: %v", err)
		}
	}
}

func TestAnonymousOnlyWhenAllowed(t *testing.T) {
	r, _ := ParseClients("", "")
	if _, ok := r.Authenticate(""); ok {
		t.Error("without clients and without AllowAnonymous nothing gets in")
	}
	r.AllowAnonymous()
	if c, ok := r.Authenticate(""); !ok || c.Name != "anonymous" || r.Enforced() {
		t.Errorf("anonymous = %+v, %v", c, ok)
	}
}

func TestLogRingAndFilter(t *testing.T) {
	l := NewLog(zerolog.New(io.Discard), 3)
	for _, e := range []Event{
		{Client: "a", Operation: "get_free_tier_status", Outcome: OutcomeOK},
		{Client: "b", Operation: "refresh_usage_snapshot", Outcome: OutcomeDenied},
		{Client: "a", Operation: "get_saturation_timeline", Outcome: OutcomeOK},
		{Client: "a", Operation: "get_quota_usage", Outcome: OutcomeError},
	} {
		l.Record(e)
	}
	all := l.Query(Filter{})
	if len(all) != 3 || all[0].Operation != "get_quota_usage" {
		t.Fatalf("ring = %+v; want the 3 newest, newest first", all)
	}
	if got := l.Query(Filter{Client: "a"}); len(got) != 2 {
		t.Errorf("client a = %d events; want 2 (the oldest one fell off)", len(got))
	}
	if got := l.Query(Filter{Outcome: OutcomeDenied}); len(got) != 1 || got[0].Client != "b" {
		t.Errorf("denied = %+v", got)
	}
	if all[0].Affected == nil {
		t.Error("affected must serialise as [], not null")
	}
}
