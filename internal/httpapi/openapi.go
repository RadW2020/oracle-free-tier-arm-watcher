package httpapi

import (
	"encoding/json"
	"reflect"
	"strings"

	"github.com/RadW2020/oracle-free-tier-arm-watcher/internal/api"
	"github.com/google/jsonschema-go/jsonschema"
)

// OpenAPI genera el documento OpenAPI 3.1 a partir del catálogo de
// operaciones y de los mismos esquemas que usan las tools MCP. No hay un
// YAML escrito a mano que se pueda quedar atrás: si una operación existe,
// está documentada.
func OpenAPI(version string) map[string]any {
	paths := map[string]any{}
	for _, op := range api.Operations {
		paths[op.Path] = map[string]any{strings.ToLower(op.Method): operationDoc(op)}
	}
	for path, doc := range legacyDocs() {
		paths[path] = doc
	}

	errSchema := mustSchema(reflect.TypeFor[api.ErrorEnvelope]())
	return map[string]any{
		"openapi": "3.1.0",
		"info": map[string]any{
			"title":   "Oracle Free Tier Watcher",
			"version": version,
			"description": "Read-only view of an Oracle Cloud Always Free tenancy for humans and agents: quota usage against the free limits, " +
				"billing risk, host saturation over time and the watcher's own health. The same operations are exposed as MCP tools at /mcp " +
				"(x-mcp-tool on each operation). Nothing here can modify the tenancy; the only side effect is refresh_usage_snapshot, which re-reads OCI " +
				"into the watcher's cache and needs the `refresh` scope.",
		},
		"servers": []any{map[string]any{"url": "/"}},
		"security": []any{
			map[string]any{"bearer": []any{}},
			map[string]any{"apiKey": []any{}},
		},
		"components": map[string]any{
			"securitySchemes": map[string]any{
				"bearer": map[string]any{"type": "http", "scheme": "bearer", "description": "A client key from API_CLIENTS (or the legacy API_KEY)."},
				"apiKey": map[string]any{"type": "apiKey", "in": "header", "name": "X-API-Key"},
			},
			"schemas": map[string]any{"ErrorEnvelope": errSchema},
		},
		"paths": paths,
	}
}

func operationDoc(op api.Operation) map[string]any {
	doc := map[string]any{
		"operationId":      op.Name,
		"summary":          op.Title,
		"description":      op.Description,
		"x-required-scope": string(op.Scope),
		"x-read-only":      op.ReadOnly,
		"x-idempotent":     op.Idempotent,
		"responses": map[string]any{
			"200": map[string]any{
				"description": "OK",
				"content":     map[string]any{"application/json": map[string]any{"schema": mustSchema(op.Output)}},
			},
			"default": map[string]any{
				"description": "Error. `error.code` is stable; `error.retryable` says whether retrying can help.",
				"content":     map[string]any{"application/json": map[string]any{"schema": map[string]any{"$ref": "#/components/schemas/ErrorEnvelope"}}},
			},
		},
	}
	if op.MCP {
		doc["x-mcp-tool"] = op.Name
	}
	input := mustSchema(op.Input)
	if op.Method == "GET" {
		props, _ := input["properties"].(map[string]any)
		required := map[string]bool{}
		if req, ok := input["required"].([]any); ok {
			for _, r := range req {
				required[r.(string)] = true
			}
		}
		var params []any
		for _, name := range sortedKeys(props) {
			p := props[name].(map[string]any)
			param := map[string]any{"name": name, "in": "query", "required": required[name], "schema": p}
			if d, ok := p["description"]; ok {
				param["description"] = d
			}
			if isArray(p) {
				param["style"], param["explode"] = "form", false
			}
			params = append(params, param)
		}
		if len(params) > 0 {
			doc["parameters"] = params
		}
	} else {
		doc["requestBody"] = map[string]any{
			"required": false,
			"content":  map[string]any{"application/json": map[string]any{"schema": input}},
		}
	}
	return doc
}

func legacyDocs() map[string]any {
	legacy := func(summary, description string, out reflect.Type) map[string]any {
		return map[string]any{"get": map[string]any{
			"tags":        []any{"legacy"},
			"summary":     summary,
			"description": description,
			"responses": map[string]any{"200": map[string]any{
				"description": "OK",
				"content":     map[string]any{"application/json": map[string]any{"schema": mustSchema(out)}},
			}},
		}}
	}
	return map[string]any{
		"/usage":  legacy("Full usage (legacy)", "Reads OCI live on every call. Kept for Checkly and humans; agents should prefer /v1. Its `status` does not account for missing data: check `complete` and `sources`.", reflect.TypeFor[UsageResponse]()),
		"/status": legacy("Quick status (legacy)", "Reads OCI live on every call. Checkly asserts $.maxUsagePercentage, so the field is always present.", reflect.TypeFor[StatusResponse]()),
		"/limits": legacy("Always Free limits (legacy)", "The hard-coded free tier limits.", reflect.TypeFor[LimitsResponse]()),
		"/health": map[string]any{"get": map[string]any{
			"tags": []any{"legacy"}, "summary": "Liveness", "security": []any{},
			"description": "Process liveness only; says nothing about OCI. Use /v1/diagnostics for that.",
			"responses":   map[string]any{"200": map[string]any{"description": "OK"}},
		}},
	}
}

func mustSchema(t reflect.Type) map[string]any {
	s, err := api.SchemaFor(t)
	if err != nil {
		panic(err)
	}
	return toMap(s)
}

func toMap(s *jsonschema.Schema) map[string]any {
	raw, _ := json.Marshal(s)
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	return m
}

func isArray(p map[string]any) bool {
	switch t := p["type"].(type) {
	case string:
		return t == "array"
	case []any:
		for _, v := range t {
			if v == "array" {
				return true
			}
		}
	}
	return false
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
}
