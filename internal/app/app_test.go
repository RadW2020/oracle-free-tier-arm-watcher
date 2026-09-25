package app

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/RadW2020/oracle-free-tier-arm-watcher/internal/api"
	"github.com/RadW2020/oracle-free-tier-arm-watcher/internal/audit"
	"github.com/RadW2020/oracle-free-tier-arm-watcher/internal/mcpserver"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rs/zerolog"
)

const clients = "agent:read:agent-key,ops:audit+read+refresh:ops-key"

func start(t *testing.T, scenario string) (*App, *httptest.Server) {
	t.Helper()
	a, err := New(Config{DataSource: "fixture", Scenario: scenario, APIClients: clients, APIKey: "checkly-key", Logger: zerolog.New(io.Discard)})
	if err != nil {
		t.Fatal(err)
	}
	_ = a.Prime(context.Background())
	srv := httptest.NewServer(a.Handler)
	t.Cleanup(srv.Close)
	return a, srv
}

func get(t *testing.T, url, key string) (int, http.Header, map[string]any) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	return do(t, req)
}

func do(t *testing.T, req *http.Request) (int, http.Header, map[string]any) {
	t.Helper()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var body map[string]any
	raw, _ := io.ReadAll(resp.Body)
	_ = json.Unmarshal(raw, &body)
	return resp.StatusCode, resp.Header, body
}

func errCode(body map[string]any) string {
	e, _ := body["error"].(map[string]any)
	code, _ := e["code"].(string)
	return code
}

func TestRESTAuthAndErrors(t *testing.T) {
	_, srv := start(t, "quiet")

	tests := []struct {
		name   string
		method string
		path   string
		key    string
		status int
		code   string
	}{
		{"no key", "GET", "/v1/status", "", 401, "unauthenticated"},
		{"bad key", "GET", "/v1/status", "nope", 401, "unauthenticated"},
		{"read key reads", "GET", "/v1/status", "agent-key", 200, ""},
		{"legacy API_KEY reads /v1 too", "GET", "/v1/status", "checkly-key", 200, ""},
		{"read key cannot refresh", "POST", "/v1/snapshot/refresh", "agent-key", 403, "permission_denied"},
		{"read key cannot audit", "GET", "/v1/audit", "agent-key", 403, "permission_denied"},
		{"wrong method", "DELETE", "/v1/status", "agent-key", 405, "method_not_allowed"},
		{"unknown route", "GET", "/v1/buckets", "agent-key", 404, "not_found"},
		{"typo in a parameter", "GET", "/v1/quotas?nameContain=media", "agent-key", 400, "invalid_argument"},
		{"unknown quota", "GET", "/v1/quotas?quota=gpus", "agent-key", 400, "invalid_argument"},
		{"bad time", "GET", "/v1/saturation?start=yesterday", "agent-key", 400, "invalid_argument"},
		{"beyond retention", "GET", "/v1/saturation?start=2026-01-01T00:00:00Z", "agent-key", 400, "out_of_range"},
		{"too many points", "GET", "/v1/saturation?start=2026-09-24T09:00:00Z", "agent-key", 400, "invalid_argument"},
		{"window too long for 1m", "GET", "/v1/saturation?start=2026-09-20T00:00:00Z&includePoints=false", "agent-key", 400, "invalid_argument"},
		{"summary only allows the long window at 1h", "GET", "/v1/saturation?start=2026-09-20T00:00:00Z&resolution=1h&includePoints=false", "agent-key", 200, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest(tt.method, srv.URL+tt.path, nil)
			if tt.key != "" {
				req.Header.Set("Authorization", "Bearer "+tt.key)
			}
			status, header, body := do(t, req)
			if status != tt.status {
				t.Fatalf("status = %d; want %d (%v)", status, tt.status, body)
			}
			if code := errCode(body); code != tt.code {
				t.Errorf("error.code = %q; want %q", code, tt.code)
			}
			if tt.code != "" {
				e := body["error"].(map[string]any)
				if _, ok := e["retryable"]; !ok {
					t.Error("every error must say whether it is retryable")
				}
			}
			if header.Get("X-Request-Id") == "" {
				t.Error("every /v1 response carries X-Request-Id")
			}
		})
	}
}

func TestRequestIDIsEchoedAndAudited(t *testing.T) {
	a, srv := start(t, "quiet")
	req, _ := http.NewRequest("GET", srv.URL+"/v1/billing-risk", nil)
	req.Header.Set("X-API-Key", "agent-key")
	req.Header.Set("X-Request-Id", "eval-run-42")
	_, header, body := do(t, req)
	if header.Get("X-Request-Id") != "eval-run-42" {
		t.Errorf("request id = %q", header.Get("X-Request-Id"))
	}
	if body["meta"].(map[string]any)["requestId"] != "eval-run-42" {
		t.Error("meta.requestId must match the header")
	}
	events := a.Audit.Query(auditFilter("agent"))
	if len(events) == 0 || events[0].RequestID != "eval-run-42" || events[0].Operation != api.OpBilling || events[0].Transport != "rest" {
		t.Errorf("audit = %+v", events)
	}
}

func TestRefreshIsTheOnlyMutationAndIsAudited(t *testing.T) {
	a, srv := start(t, "quiet")
	post := func() map[string]any {
		req, _ := http.NewRequest("POST", srv.URL+"/v1/snapshot/refresh", strings.NewReader(`{"reason":"test"}`))
		req.Header.Set("Authorization", "Bearer ops-key")
		status, _, body := do(t, req)
		if status != 200 {
			t.Fatalf("refresh = %d %v", status, body)
		}
		return body
	}
	// Prime ya leyó: la primera petición cae en el cooldown.
	if b := post(); b["refreshed"] != false || b["reason"] != "cooldown" || b["retryAfterSeconds"] == nil {
		t.Errorf("refresh right after a read = %v; want cooldown", b)
	}
	events := a.Audit.Query(auditFilter("ops"))
	if len(events) != 1 || len(events[0].Affected) != 0 || events[0].Args["reason"] != "test" {
		t.Errorf("audit = %+v; a cooldown changes nothing", events)
	}

	req, _ := http.NewRequest("POST", srv.URL+"/v1/snapshot/refresh", strings.NewReader(`{"reasn":"typo"}`))
	req.Header.Set("Authorization", "Bearer ops-key")
	if status, _, body := do(t, req); status != 400 || errCode(body) != "invalid_argument" {
		t.Errorf("unknown body field = %d %v", status, body)
	}
}

func TestTimelineRateLimit(t *testing.T) {
	_, srv := start(t, "incident-2026-09-17")
	url := srv.URL + "/v1/saturation?start=2026-09-17T15:00:00Z&end=2026-09-17T15:30:00Z&metrics=ingress_throttle_drops"
	limited := false
	for i := 0; i < 15; i++ {
		status, header, body := get(t, url, "agent-key")
		if status == 429 {
			limited = true
			if errCode(body) != "rate_limited" || header.Get("Retry-After") == "" {
				t.Errorf("429 without rate_limited code or Retry-After: %v", body)
			}
			break
		}
	}
	if !limited {
		t.Error("15 timeline calls in a burst should hit the per-client limit")
	}
	// El límite es por cliente: otro cliente sigue pudiendo consultar.
	if status, _, _ := get(t, url, "ops-key"); status != 200 {
		t.Errorf("another client got %d", status)
	}
}

func TestScenarioOutcomes(t *testing.T) {
	t.Run("unknown is not OK", func(t *testing.T) {
		_, srv := start(t, "egress-unavailable")
		_, _, status := get(t, srv.URL+"/v1/status", "agent-key")
		if status["status"] != "UNKNOWN" || status["complete"] != false {
			t.Errorf("status = %v %v", status["status"], status["complete"])
		}
		_, _, billing := get(t, srv.URL+"/v1/billing-risk", "agent-key")
		if billing["verdict"] != "UNKNOWN" {
			t.Errorf("billing verdict = %v", billing["verdict"])
		}
		// La API heredada no cambia su status, pero lo avisa.
		_, _, legacy := get(t, srv.URL+"/status", "agent-key")
		if legacy["status"] != "OK" || legacy["complete"] != false {
			t.Errorf("legacy = %v", legacy)
		}
	})

	t.Run("projection sees what the level does not", func(t *testing.T) {
		_, srv := start(t, "egress-at-risk")
		_, _, status := get(t, srv.URL+"/v1/status", "agent-key")
		_, _, billing := get(t, srv.URL+"/v1/billing-risk", "agent-key")
		if status["status"] != "OK" || billing["verdict"] != "AT_RISK" {
			t.Fatalf("status %v / billing %v", status["status"], billing["verdict"])
		}
		for _, q := range billing["quotas"].([]any) {
			q := q.(map[string]any)
			if q["quota"] == "egress" && q["crossesLimitOn"] != "2026-09-26" {
				t.Errorf("egress = %v", q)
			}
		}
	})

	t.Run("incident drops are found", func(t *testing.T) {
		_, srv := start(t, "incident-2026-09-17")
		_, _, tl := get(t, srv.URL+"/v1/saturation?start=2026-09-17T15:00:00Z&end=2026-09-17T15:40:00Z&metrics=ingress_throttle_drops,ingress_bytes", "agent-key")
		intervals := tl["summary"].(map[string]any)["throttleDropIntervals"].([]any)
		if len(intervals) != 1 {
			t.Fatalf("intervals = %v", intervals)
		}
		iv := intervals[0].(map[string]any)
		if iv["start"] != "2026-09-17T15:14:00Z" || iv["end"] != "2026-09-17T15:20:00Z" || iv["totalDrops"] != 81000.0 {
			t.Errorf("interval = %v", iv)
		}
		if tl["bucketLabel"] != "end_of_interval" {
			t.Error("the bucket labelling must be explicit")
		}
	})

	t.Run("ambiguous names return every match", func(t *testing.T) {
		_, srv := start(t, "quiet")
		_, _, q := get(t, srv.URL+"/v1/quotas?nameContains=MEDIA", "agent-key")
		if q["matches"] != 2.0 {
			t.Errorf("matches = %v; want postiz-media and media-backups", q["matches"])
		}
	})

	t.Run("not configured", func(t *testing.T) {
		_, srv := start(t, "not-configured")
		status, _, body := get(t, srv.URL+"/v1/status", "agent-key")
		if status != 503 || errCode(body) != "not_configured" {
			t.Errorf("status = %d %v", status, body)
		}
		_, _, diag := get(t, srv.URL+"/v1/diagnostics", "agent-key")
		if diag["health"] != "degraded" || diag["configured"] != false {
			t.Errorf("diagnostics = %v", diag)
		}
		// El contrato heredado de NOT_CONFIGURED no cambia.
		code, _, legacy := get(t, srv.URL+"/status", "agent-key")
		if code != 503 || legacy["status"] != "NOT_CONFIGURED" {
			t.Errorf("legacy = %d %v", code, legacy)
		}
	})
}

func TestLegacyAuthKeepsItsMessages(t *testing.T) {
	_, srv := start(t, "quiet")
	if status, _, body := get(t, srv.URL+"/health", ""); status != 200 || body["status"] != "ok" {
		t.Errorf("/health must stay public: %d", status)
	}
	req, _ := http.NewRequest("GET", srv.URL+"/usage", nil)
	if status, _, body := do(t, req); status != 401 || body["error"] != "Missing X-API-Key header" {
		t.Errorf("missing = %d %v", status, body)
	}
	req.Header.Set("X-API-Key", "wrong")
	if status, _, body := do(t, req); status != 401 || body["error"] != "Invalid API key" {
		t.Errorf("invalid = %d %v", status, body)
	}
	req.Header.Set("X-API-Key", "checkly-key")
	status, _, body := do(t, req)
	if status != 200 || body["configured"] != true {
		t.Fatalf("checkly key = %d", status)
	}
	// Las rutas que asierta Checkly.
	if _, ok := body["maxUsagePercentage"]; !ok {
		t.Error("$.maxUsagePercentage missing")
	}
	if _, ok := body["usage"].(map[string]any)["bandwidth"].(map[string]any)["percentage"]; !ok {
		t.Error("$.usage.bandwidth.percentage missing")
	}
}

func TestOpenAPICoversEveryOperation(t *testing.T) {
	_, srv := start(t, "quiet")
	status, _, doc := get(t, srv.URL+"/openapi.json", "")
	if status != 200 || doc["openapi"] != "3.1.0" {
		t.Fatalf("openapi = %d", status)
	}
	paths := doc["paths"].(map[string]any)
	for _, op := range api.Operations {
		item, ok := paths[op.Path].(map[string]any)
		if !ok {
			t.Errorf("%s missing from OpenAPI", op.Path)
			continue
		}
		o := item[strings.ToLower(op.Method)].(map[string]any)
		if o["operationId"] != op.Name || o["x-required-scope"] != string(op.Scope) {
			t.Errorf("%s: %v", op.Path, o["operationId"])
		}
	}
	for _, legacy := range []string{"/usage", "/status", "/limits", "/health"} {
		if _, ok := paths[legacy]; !ok {
			t.Errorf("legacy %s missing from OpenAPI", legacy)
		}
	}
	params := paths["/v1/saturation"].(map[string]any)["get"].(map[string]any)["parameters"].([]any)
	found := false
	for _, p := range params {
		p := p.(map[string]any)
		if p["name"] == "resolution" {
			enum := p["schema"].(map[string]any)["enum"].([]any)
			found = len(enum) == 3
		}
	}
	if !found {
		t.Error("resolution must be discoverable as an enum in the OpenAPI parameters")
	}
}

// ---------------------------------------------------------------------------
// MCP
// ---------------------------------------------------------------------------

func connect(t *testing.T, srv *httptest.Server, key string) *mcp.ClientSession {
	t.Helper()
	session, err := mcpserver.Connect(context.Background(), srv.URL+"/mcp", key)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}

func TestMCPToolsAreDescribedForAgents(t *testing.T) {
	_, srv := start(t, "quiet")
	session := connect(t, srv, "agent-key")

	if got := session.InitializeResult().Instructions; !strings.Contains(got, "never report those as 0") && !strings.Contains(got, "Never report those as 0") {
		t.Errorf("server instructions must tell agents how to read unknown values: %q", got)
	}

	res, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{api.OpStatus: true, api.OpQuotas: true, api.OpBilling: true, api.OpSaturation: true, api.OpDiagnostics: true, api.OpRefresh: true}
	if len(res.Tools) != len(want) {
		t.Errorf("tools = %d; want %d", len(res.Tools), len(want))
	}
	for _, tool := range res.Tools {
		if !want[tool.Name] {
			t.Errorf("unexpected tool %q (the audit log is REST-only on purpose)", tool.Name)
		}
		if len(tool.Description) < 150 {
			t.Errorf("%s: description too thin for an agent to choose it", tool.Name)
		}
		if tool.OutputSchema == nil || tool.Annotations == nil {
			t.Errorf("%s: missing output schema or annotations", tool.Name)
			continue
		}
		if tool.Annotations.DestructiveHint == nil || *tool.Annotations.DestructiveHint {
			t.Errorf("%s: no tool here is destructive", tool.Name)
		}
		if tool.Annotations.ReadOnlyHint != (tool.Name != api.OpRefresh) {
			t.Errorf("%s: readOnlyHint = %v", tool.Name, tool.Annotations.ReadOnlyHint)
		}
	}
}

func TestMCPRejectsWithoutKey(t *testing.T) {
	_, srv := start(t, "quiet")
	if _, err := mcpserver.Connect(context.Background(), srv.URL+"/mcp", ""); err == nil {
		t.Error("an MCP client without a key must not even initialize")
	}
	if _, err := mcpserver.Connect(context.Background(), srv.URL+"/mcp", "wrong"); err == nil {
		t.Error("an MCP client with a wrong key must not initialize")
	}
}

func callTool(t *testing.T, session *mcp.ClientSession, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return res
}

func TestMCPErrorsAreStructuredAndActionable(t *testing.T) {
	a, srv := start(t, "quiet")
	session := connect(t, srv, "agent-key")

	res := callTool(t, session, api.OpRefresh, map[string]any{"reason": "want fresh data"})
	if !res.IsError {
		t.Fatal("a read-only key must not be able to refresh")
	}
	var env api.ErrorEnvelope
	if err := json.Unmarshal([]byte(res.Content[0].(*mcp.TextContent).Text), &env); err != nil {
		t.Fatalf("error content is not the JSON envelope: %v", err)
	}
	if env.Error.Code != api.CodePermissionDenied || env.Error.RequiredScope != "refresh" {
		t.Errorf("error = %+v", env.Error)
	}

	res = callTool(t, session, api.OpSaturation, map[string]any{"start": "2026-09-25T08:00:00Z", "resolution": "2m"})
	if !res.IsError {
		t.Error("an invalid enum value must be rejected")
	}

	events := a.Audit.Query(auditFilter("agent"))
	if len(events) == 0 || events[len(events)-1].Transport != "mcp" || events[len(events)-1].Outcome != "denied" {
		t.Errorf("audit = %+v", events)
	}
}

// TestRESTAndMCPAgree: dos transportes, una respuesta.
func TestRESTAndMCPAgree(t *testing.T) {
	_, srv := start(t, "incident-2026-09-17")
	session := connect(t, srv, "agent-key")

	cases := []struct {
		tool string
		args map[string]any
		path string
	}{
		{api.OpStatus, nil, "/v1/status"},
		{api.OpQuotas, map[string]any{"quota": "object_storage"}, "/v1/quotas?quota=object_storage"},
		{api.OpBilling, nil, "/v1/billing-risk"},
		{api.OpSaturation, map[string]any{"start": "2026-09-17T15:00:00Z", "end": "2026-09-17T15:30:00Z", "metrics": []string{"ingress_throttle_drops"}}, "/v1/saturation?start=2026-09-17T15:00:00Z&end=2026-09-17T15:30:00Z&metrics=ingress_throttle_drops"},
		{api.OpDiagnostics, nil, "/v1/diagnostics"},
	}
	for _, c := range cases {
		t.Run(c.tool, func(t *testing.T) {
			res := callTool(t, session, c.tool, c.args)
			if res.IsError {
				t.Fatalf("mcp error: %v", res.Content)
			}
			raw, _ := json.Marshal(res.StructuredContent)
			var viaMCP map[string]any
			_ = json.Unmarshal(raw, &viaMCP)
			_, _, viaREST := get(t, srv.URL+c.path, "agent-key")

			stripVolatile(viaMCP)
			stripVolatile(viaREST)
			a, _ := json.Marshal(viaMCP)
			b, _ := json.Marshal(viaREST)
			if string(a) != string(b) {
				t.Errorf("MCP and REST differ:\nmcp:  %s\nrest: %s", a, b)
			}
		})
	}
}

func stripVolatile(m map[string]any) {
	if meta, ok := m["meta"].(map[string]any); ok {
		delete(meta, "requestId")
	}
	if snap, ok := m["snapshot"].(map[string]any); ok {
		delete(snap, "lastAttemptAt")
		delete(snap, "lastSuccessAt")
	}
}

func TestMCPTimelineHonoursTheFixtureClock(t *testing.T) {
	_, srv := start(t, "incident-2026-09-17")
	session := connect(t, srv, "agent-key")
	res := callTool(t, session, api.OpSaturation, map[string]any{"start": time.Date(2026, 9, 17, 17, 30, 0, 0, time.UTC).Format(time.RFC3339), "metrics": []string{"cpu_percent"}})
	if res.IsError {
		t.Fatalf("%v", res.Content)
	}
	raw, _ := json.Marshal(res.StructuredContent)
	var out api.SaturationResult
	_ = json.Unmarshal(raw, &out)
	if out.DataLag.LagSeconds == nil || *out.DataLag.LagSeconds < 180 {
		t.Errorf("data lag = %+v; the window reaches now, so the lag must be reported", out.DataLag)
	}
	if out.Meta.DataSource != "fixture" || out.Meta.Scenario != "incident-2026-09-17" {
		t.Errorf("meta = %+v; fixture data must be labelled as such", out.Meta)
	}
}

func auditFilter(client string) audit.Filter { return audit.Filter{Client: client} }
