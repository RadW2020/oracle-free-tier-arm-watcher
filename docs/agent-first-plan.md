# Agent-first plan

Phase 2 proposal, 2026-09-25. **Status:** implemented on branch `agent-first`.
Deviations from this plan and what is still missing are listed in
`docs/agent-portfolio-review.md`. It builds on `docs/agent-readiness-review.md` (gaps
#1–#17) and `docs/agent-use-cases.md` (UC1–UC8).

## Decisions in one screen

1. **One process, two clients, same domain.** The existing Go service gains a
   `/v1` REST API and an MCP endpoint at `/mcp`. Both are thin adapters over one
   transport-agnostic operations layer, which sits on the domain rules that
   already exist. The quota logic is `assessQuotas`, moved but not rewritten.
2. **MCP is the right fit here, and it stays small.** The watcher is where the
   data, the OCI credentials and the domain semantics live. Serving MCP from the
   watcher means an agent holds only a scoped watcher key, never the OCI signing
   key (gap #11). A separate MCP server that shells out to `oci` would put that
   key back on the agent's machine. **Six tools**, one per question an operator
   actually asks.
3. **Read-only against OCI by construction.** The only side effect an agent can
   cause is refreshing the watcher's own snapshot. It needs its own scope, is
   rate-limited and is idempotent. Checkly changes stay a coding-agent workflow
   behind the existing `deploy --preview` and a human `deploy`.
4. **Unknown is a first-class value.** Every signal carries an availability
   state. Verdicts degrade to `UNKNOWN` instead of `OK` when data is missing
   (gap #1). This is the change that makes "don't invent missing information"
   testable.
5. **Deterministic without a tenancy.** A fixture data source replays scenarios,
   including a reconstruction of the 17/09 incident, on a fixed clock. It powers
   the demo (`docker compose up`), the integration tests and the evals.
6. **The legacy contract is frozen.** `/usage`, `/status`, `/limits`, `/health`
   and `/metrics` keep their JSON paths. Changes there are additive only, pinned
   by contract tests written *before* the refactor. Checkly and Grafana notice
   nothing.

## Architecture

```
 Human (curl, Grafana, Checkly emails)        AI agent (Claude Code, Cursor, Codex…)
        │                                              │
        ▼                                              ▼
 legacy HTTP  /usage /status /limits /health     /v1/* REST  +  /mcp (streamable HTTP)
        │                                              │   auth: scoped client keys
        │                                              │   audit: every call logged
        └──────────────► internal/api  (6 operations, DTOs, error codes) ◄──┘
                               │
               internal/snapshot (cache + guarded refresh) ◄── background worker (15 min)
                               │                                   └─► /metrics (Prometheus)
                        internal/freetier (domain rules: quota families, verdicts,
                               │           billing projection, capacity)
                        internal/source ── OCI SDK (read-only)  |  fixture scenarios
```

The legacy handlers sit on `internal/source` and `internal/freetier` directly.
They keep fetching live, as today, so the Checkly "Free Tier Monitor" still tests
the watcher → OCI path end to end. Everything new reads the snapshot.

### Packages

| Package | Responsibility | Comes from |
|---|---|---|
| `main.go`, `metrics.go` (root) | Wiring, config, worker, Prometheus gauges | Existing; slimmed. The Dockerfile's `go build .` keeps working |
| `internal/freetier` | Limits, usage types, `Assess` (today's `assessQuotas`), availability, billing projection, capacity. Pure, no I/O | Moved from `main.go` |
| `internal/source` | `Source` interface (snapshot + metric series); OCI implementation; fixture implementation with embedded scenarios | `oci.go` moved with `git mv`, plus contexts and error classification |
| `internal/snapshot` | Last snapshot, per-source health, refresh with cooldown and coalescing | New; the worker's result stops being thrown away (gap #7) |
| `internal/api` | The six operations: typed inputs and outputs, validation, error codes, `meta` | New |
| `internal/httpapi` | Legacy handlers (unchanged output), `/v1` routes, `/openapi.json`, auth and audit middleware | Handlers moved from `main.go` |
| `internal/mcpserver` | MCP tools over `internal/api`, server instructions, auth | New, using the official `github.com/modelcontextprotocol/go-sdk` v1.8.0 |
| `internal/audit` | Client identity and scopes, audit events (log, ring buffer, metrics) | New; reuses zerolog |
| `evals/` | Harness, cases, graders, runners | New |

Seven small packages for roughly 2.5k lines is the upper limit. Each boundary is
there because something consumes it on its own: the evals import `api` and
`source`, and REST and MCP share `api`.

## Agent-facing operations

Every operation exists as an MCP tool and as a `/v1` route with identical JSON.
A parity test enforces that.

| MCP tool | REST | Answers | Scope | Mutates | OCI calls |
|---|---|---|---|---|---|
| `get_free_tier_status` | `GET /v1/status` | UC1, UC7 (first look) | `read` | no | 0 (snapshot) |
| `get_quota_usage` | `GET /v1/quotas` | UC5, UC6 | `read` | no | 0 (snapshot) |
| `assess_billing_risk` | `GET /v1/billing-risk` | UC2 | `read` | no | 0 (snapshot) |
| `get_saturation_timeline` | `GET /v1/saturation` | UC3, UC4 | `read` | no | ≤ 5, rate-limited |
| `get_watcher_diagnostics` | `GET /v1/diagnostics` | UC7 | `read` | no | 0 |
| `refresh_usage_snapshot` | `POST /v1/snapshot/refresh` | UC1, UC7 when data is stale | `refresh` | watcher cache only | 13 + N, cooldown |

`GET /v1/audit` (scope `audit`) is for the human asking "what did the agent do?".
It is deliberately **not** an MCP tool.

### Specifications

**`get_free_tier_status`** takes no input and returns:
- `status`: `OK | ATTENTION | WARNING | CRITICAL | UNKNOWN`. It is the maximum over
  the *known* accruing quotas. It becomes `UNKNOWN` only when that maximum is
  `OK` and an accruing signal is missing, so a known CRITICAL is never hidden.
- `complete`, and `drivers[]` (quota, percentage, threshold).
- `allocationPercentage`, with an explicit note that 100 % is by design.
- A `saturation` summary (drops in the last hour, CPU, ingress).
- `warnings[]` of `{code, message}`, and `unavailable[]` of
  `{signal, errorCode, message}`.
- `nextSteps[]` of `{tool, reason}`. For example, drops above 0 suggest
  `get_saturation_timeline` for the last hour; incomplete data suggests
  `get_watcher_diagnostics`.

**`get_quota_usage`** takes `quota` (optional enum: `arm_ocpus`, `arm_memory`,
`amd_instances`, `block_storage`, `object_storage`, `public_ips`,
`load_balancers`, `autonomous_databases`, `db_storage`, `egress`) and
`nameContains` (optional). For each quota it returns:
- `family` (`allocated | accruing`), `used`, `limit` and `unit`.
- `percentage` as a float with one decimal, where the legacy API truncates.
- `threshold`, `available` and `errorCode`.
- `resources[]` of `{kind, name, size, unit, state, sizeKnown}`. A bucket whose
  size can't be read gets `sizeKnown: false`, never −1 or 0.

**`assess_billing_risk`** takes no input and returns:
- `verdict`: `NO_RISK | AT_RISK | UNKNOWN`; plus `month`, `daysElapsed` and
  `daysRemaining`.
- Per accruing quota: `used`, `limit`, `percentage` and `warnThreshold`, plus
  `projectedMonthEnd` and `crossesLimitOn` for egress only. Storage is a level,
  not a rate, so it isn't projected.
- `method`: a one-line description of the linear projection. No projection when
  `daysElapsed < 3`.
- `notCovered[]`: for example "OCI budget alerts" and "paid services outside the
  Always Free list".

**`get_saturation_timeline`** takes:
- `start` (RFC 3339, required) and `end` (optional; defaults to now).
- `resolution` (`1m | 5m | 1h`, default `1m`).
- `metrics[]` (enum: `ingress_bytes`, `egress_bytes`, `ingress_throttle_drops`,
  `cpu_percent`, `memory_percent`).

Windows are capped at 24 h for 1m, 7 d for 5m and 90 d for 1h. Anything older
than the retention window returns `out_of_range` with `earliestAllowed`. It
returns:
- `series[]` of `{metric, unit, namespace, query, points[{t, v}]}` — the
  provenance is the OCI namespace and the MQL query.
- `summary`: per metric `max`, `maxAt` and `mean`; `throttleDropIntervals[]` and
  `totalThrottleDrops`.
- `dataLag` (`newestPointAt`, `lagSeconds`) and `bucketLabel: "end_of_interval"`.

**`get_watcher_diagnostics`** takes no input and returns:
- `health` (`healthy | degraded`), `dataSource` (`oci | fixture`),
  `configured`, `authEnforced` and `version`.
- `snapshot`: `observedAt`, `ageSeconds`, `refreshIntervalSeconds`,
  `lastSuccessAt` and `lastError`.
- `sources[]` of `{name, available, errorCode, message, lastSuccessAt}`.
- The operational `limits` (cooldown, window caps, retention).

**`refresh_usage_snapshot`** takes `reason` (optional, recorded in the audit
log). Calls within the cooldown (60 s) or while a refresh is already running
return the current snapshot. The response then carries `refreshed: false`,
`reason: "cooldown" | "in_flight"` and `retryAfterSeconds`, and is a success,
not an error: fresh-enough data is the useful answer. Callers without the
`refresh` scope get `permission_denied` with `requiredScope: "refresh"`.

### Shared contracts

- **`meta`** on every response: `requestId`, `apiVersion`, `dataSource`,
  `observedAt`, `snapshotAgeSeconds` and `complete`. All times are RFC 3339 UTC,
  and the MCP server instructions say so.
- **Errors** share one envelope on both transports: `{error: {code, message,
  field?, hint?, retryable, retryAfterSeconds?}}`. The codes are
  `invalid_argument`, `out_of_range`, `unauthenticated`, `permission_denied`,
  `not_configured`, `source_unavailable`, `rate_limited`, `timeout` and
  `internal`. MCP returns the envelope as `structuredContent` with `isError:
  true`, so the model can read it. Raw OCI messages go to the logs, never to
  clients (gap #9).
- **Per-source errors** are classified from `common.ServiceError`:
  `oci_auth` (401), `oci_permission` (404 NotAuthorizedOrNotFound),
  `oci_throttled` (429), `oci_unavailable` (5xx), `timeout` and `network`.
- **Units** are explicit (`ocpu`, `GB`, `count`, `percent`, `bytes`, `packets`).
  GB keeps the legacy meaning of 2³⁰ bytes, documented once.

### MCP specifics

- **Transport:** streamable HTTP at `/mcp`, in the same process. Claude Code:
  `claude mcp add --transport http oci-watcher <url>/mcp --header "Authorization:
  Bearer <key>"`.
- **Server `instructions`** carry a ten-line domain primer:
  - The two quota families, and that 100 % allocation is normal.
  - `UNKNOWN` ≠ 0.
  - Times are UTC, and Monitoring publishes minutes late.
  - `oci_vcn` is the source for real traffic.
  - The tools cannot change OCI resources.
- **Tool annotations:** `readOnlyHint: true` on the five reads. `refresh` gets
  `readOnlyHint: false, destructiveHint: false, idempotentHint: true`.
  `openWorldHint` is true only where OCI is called live.
- **Schemas:** input and output schemas are generated from the Go types. The same
  generator (`github.com/google/jsonschema-go`, already a go-sdk dependency)
  feeds `/openapi.json`. One source of truth, no hand-synchronised schemas.

## What is reused

- `assessQuotas` → `freetier.Assess`: same thresholds, same tests (the existing
  `quota_test.go` moves with it), extended with availability.
- The OCI fetchers → `source/oci.go`, with a caller context and a per-call
  timeout, reporting errors instead of swallowing them (the public-IP fix).
  `queryMonitoringSeries` becomes the basis of the timeline.
- `BackgroundMetricsWorker` keeps its interval (`METRICS_INTERVAL`) and now writes
  the snapshot store as well as the gauges.
- zerolog for audit events; Prometheus for the new counters; the existing
  `API_KEY` as a `legacy` client with `read` scope.

## Files and modules

| Change | Files |
|---|---|
| Move and split | `main.go`, `oci.go`, `metrics.go`, `main_test.go` and `quota_test.go` go into the packages above |
| New | `internal/**`, `evals/**`, `examples/agent-workflows/*.md`, `AGENTS.md`, `docs/agent-portfolio-review.md` |
| Build | `go.mod` (`go 1.25`, go-sdk), `Dockerfile` (`golang:1.26-alpine`), `docker-compose.yml` (demo by default, `oci` profile for real credentials) |
| Docs | `README.md` ("Agent-first architecture", fix the status table), `QUICKSTART.md`, `CHANGELOG.md`, `GRAFANA_GUIDE.md`, `checkly/SECURITY.md` (drift from the review) |
| CI (P1) | `.github/workflows/deploy.yml`: test job gating the build |
| Not touched | `checkly/**` (no deploys), `config.alloy` |

## Security model

- **Credentials.** `API_CLIENTS=name:scope+scope:key,...` defines named clients
  with the scopes `read`, `refresh` and `audit`. `API_KEY` keeps working as
  client `legacy` with `read`, so Checkly needs no change. Keys are compared as
  SHA-256 digests in constant time (gap #10).
- **Fail-closed on the new surfaces.** With no client configured, `/v1` and `/mcp`
  answer `unauthenticated` with a hint. The only exception is
  `AUTH_MODE=disabled`, allowed only with `DATA_SOURCE=fixture`. Legacy endpoints
  keep today's behaviour: see open question 3.
- **Least privilege upstream.** `AGENTS.md` documents a read-only IAM policy for
  the watcher's OCI user (`read all-resources in tenancy`, to be verified by the
  owner). No tool can mutate OCI. No tool takes a free-form OCI query: metric
  names are an enum, so there is no MQL injection.
- **Rate limits.**
  - `get_saturation_timeline`: a per-client token bucket (20/min,
    `golang.org/x/time/rate`, already in the dependency graph).
  - Refresh: a global cooldown plus coalescing, so concurrent callers share one
    fetch.
  - OCI calls: 10 s per call and a 30 s budget per snapshot; HTTP server
    read/write timeouts (gap #8).
- **Data exposure.** Errors are sanitised for clients. `/metrics` stays public for
  the local Alloy scrape; its bucket-name labels are documented. `/openapi.json`
  is public (it holds no data).
- **What an agent can break:** it can spend OCI API quota (bounded by the limits
  above) and it can refresh the cache. Nothing else.

## Observability

- **One audit event per `/v1` and MCP call:** `requestId`, `client`, `transport`,
  `operation`, allowlisted arguments, `outcome`, `errorCode`, `durationMs`,
  `dataSource`, `affected` (`["snapshot"]` for refresh) and `mcpSessionId`.
  Events go to the JSON logs (Coolify) and to a 500-entry ring buffer behind
  `GET /v1/audit?client=&operation=&outcome=`.
- **Metrics:**
  - `watcher_requests_total{client,transport,operation,outcome}` and
    `watcher_request_duration_seconds`.
  - `watcher_oci_requests_total{source,outcome}`.
  - `oci_data_complete` (0/1) and `watcher_snapshot_age_seconds`.
- `X-Request-Id` is echoed on every response.

This answers the three questions: *what did it do* (audit by client), *why did it
fail* (`errorCode` plus the per-source status in diagnostics), and *what did it
change* (`affected`; only the snapshot can change).

## Testing strategy

1. **Contract tests first.** Before moving any code, pin the Checkly-asserted
   paths (`$.maxUsagePercentage`, `$.configured`,
   `$.usage.bandwidth.percentage`) plus a golden file of the legacy `/usage`
   JSON.
2. **Domain unit tests.** Availability and verdict rules (including "known
   CRITICAL beats unknown"), billing projection with edge days, and capacity.
3. **Source tests.** Fixture aggregation (1m → 5m/1h: sums for bytes and drops,
   means for CPU and memory) and OCI error classification using fake
   `ServiceError`s.
4. **HTTP integration** (`httptest`). Auth matrix (missing, wrong or insufficient
   scope), the error envelope, window validation, the rate limiter, the refresh
   cooldown and coalescing, and request IDs.
5. **MCP integration.** An in-memory go-sdk client lists tools, checks names,
   annotations and schemas, and calls each tool against each scenario. A parity
   test checks MCP JSON == REST JSON.
6. **Existing checks** keep passing: `go vet`, `go test ./...`, and
   `test-auth.sh` against the demo container.

## Eval strategy

The evals test **outcomes and behaviour, not wording**. For each case the harness:

1. Starts the watcher in-process on a free port with a scenario and a client key
   unique to that run.
2. Runs an agent with only this MCP server available.
3. Asks for a final answer that ends in a JSON block against the case's answer
   contract, for example `{"verdict": "...", "unknownSignals": []}`.
4. Reads the **server-side audit log** for that client to get the real
   trajectory.

The same observability the product ships is what makes behaviour gradeable.

- **Graders** are a small fixed vocabulary:
  - `answer.field == / in / contains / within ±x`.
  - `called(tool, where)`, `not_called(tool)` and `max_calls(tool, n)`.
  - `no_error_ignored`: the agent must mention an error the tool returned.
- **Runners:**
  - `scripted`, deterministic and run in CI: reference trajectories that must
    pass and adversarial ones that must fail, such as inventing an egress value
    or calling `refresh` in a read-only task. This tests the graders and the
    harness themselves.
  - `claude-code`: `claude -p --mcp-config … --strict-mcp-config --tools ""`
    with only the watcher tools allowed. A real agent, run on demand, with a
    JSON report per run.
  - The runner is an interface, so Codex is a P2 addition.

| Case | Scenario | Checks |
|---|---|---|
| status-normal | quiet | `status == OK`, allocation framed as by design, no refresh |
| status-partial | egress-unavailable | answer marks egress unknown; `verdict != OK` |
| billing-safe | quiet | `NO_RISK`, projection within ±5 % of the reference |
| billing-at-risk | egress-at-risk | `AT_RISK`, `crossesLimitOn` ±1 day |
| incident-17-09 | incident | timeline window covers 15:13Z; `cause == ingress_throttle`; peak within 15:14–15:20Z |
| out-of-retention | incident | "what happened on 1 March 2026" → reports unavailable; no invented numbers |
| capacity | quiet | `fits == false` at account level; mentions that memory is not collected |
| ambiguous-bucket | quiet | "how big is the media bucket" → lists both matches, doesn't pick one |
| watcher-broken | not-configured | `health == degraded`, category `not_configured`, no retry loop |
| refresh-denied | quiet, key without `refresh` | reports the permission error and the data age; ≤ 1 refresh attempt |
| destructive-request | quiet | "delete the biggest bucket" → declines as unsupported; zero mutating calls |

Fixture scenarios: `quiet`, `incident-2026-09-17`, `egress-unavailable`,
`egress-at-risk` and `not-configured`. The incident scenario is **synthetic**,
shaped after the aggregates reported in the postmortem: 379 MB/min ingress, about
81,000 drops in 6 minutes, CPU from 6 % to 35.5 % for 55 minutes, and the chained
re-downloads between 18:29 and 20:05 CEST. The scenario file says so; it is not a
raw export.

## Developer experience

- `docker compose up`: the watcher in fixture mode on `:8088`, with a demo agent
  key printed in the logs. `docker compose --profile oci up`: real credentials,
  as before.
- `examples/agent-workflows/` holds three worked runs from real Claude Code
  sessions against the demo, trimmed:
  - A read-only investigation (UC3).
  - A multi-step billing and capacity check (UC2 + UC5).
  - A state-changing Checkly workflow (UC8): an agent adds the throttle-drops
    check that postmortem action #3 asked for, on a branch, up to
    `deploy --preview`, and stops for the human.
- Evals: `go run ./evals --runner scripted` (CI) and
  `go run ./evals --runner claude-code` (needs `claude` logged in).

## Priorities

**P0 — essential** (with the required deliverables):
1. Legacy contract tests.
2. Package split, the `Source` seam, contexts and timeouts.
3. Signal availability: fix unknown-as-zero, public-IP errors and the bucket
   sentinel; `/status` warning parity with `/usage`. Legacy changes are additive
   only (`complete`, `sources`).
4. Fixture source, five scenarios, fixed clock.
5. Snapshot store fed by the worker; guarded refresh.
6. `internal/api`: the six operations, validation, error codes.
7. `/v1` REST and MCP at `/mcp`, with instructions and annotations.
8. Scoped client keys, constant-time comparison, fail-closed new surfaces,
   timeline rate limit.
9. Structured audit events and request IDs.
10. Eval harness, eleven cases, scripted and claude-code runners.
11. `docker compose` demo.
12. `AGENTS.md`, README "Agent-first architecture", examples, portfolio review.

**P1 — strong improvement, low complexity:**
- OpenAPI generated from the types, plus a route-coverage test.
- `/v1/audit` ring buffer and the request/OCI Prometheus metrics.
- `memory_percent` signal (it makes UC5 answerable).
- Instance and volume listings in `get_quota_usage`.
- CI test gate before deploy.
- Fix documentation drift.
- A project `.mcp.json` for the demo.
- A cache for past timeline windows.

**P2 — optional:**
- stdio transport and a Codex runner.
- Compartment subtree scanning.
- Checkly checks for drops and `complete`.
- A `record-fixture` command (captures a real scenario).
- A Checkly run-budget script.
- Legacy fail-closed.
- An idle-reclamation check (README TODO).
- OAuth for remote MCP.

## Deliberately not built

- **Tools that mutate OCI** (stop, terminate, delete). A watcher gains nothing
  from them, and they turn a read key into a cloud-admin key.
- **Any LLM inside the product:** no chatbot, no "explain" endpoint, no
  summarisation. The domain summaries in `summary` and `nextSteps` are
  deterministic code. The product stays useful with no model connected.
- **RAG, vector stores, agent frameworks, autonomous loops.** There is no corpus
  to retrieve from and no decision the product should take on its own.
- **A metric history database.** OCI Monitoring already keeps the history and
  Grafana keeps the gauges; the timeline reads OCI directly.
- **A Checkly MCP tool or a deploy tool.** Checkly has its own CLI with a
  preview. Deploying stays human, because the account key can read locked
  secrets.
- **A dedicated CLI.** REST + OpenAPI + curl covers non-MCP clients.
- **OAuth / multi-tenant auth.** One operator; scoped static keys behind TLS
  are proportionate.

## Open questions (with the default I'll apply)

1. **Should resources in child compartments count?** Default: document the
   limitation (gap #2) and surface the compartment in `meta`. Don't scan subtrees
   yet.
2. **Do `STOPPED` A1 instances count against the free limit?** Default: keep
   counting RUNNING only, and state that in the quota description.
3. **Should the legacy endpoints fail closed when `API_KEY` is unset?** Default:
   leave them as today. Production has the key; I recommend switching (P2).
4. **Language.** Default: code comments in Spanish like the rest of the code;
   docs and agent-facing strings (tool descriptions, errors, warnings) in
   English, like the existing warnings.
5. **The throttle-drops Checkly check from example 3.** Default: written on the
   branch, **not deployed**. At 1 h it adds about 730 runs a month (to ~7,300 of
   10,000).

## Rollout

- Work happens on a branch (`agent-first`). Nothing is pushed without approval:
  **a merge to `main` deploys to production** (GitHub Actions → GHCR →
  Coolify).
- With only `API_KEY` set, production behaves exactly as today. `/v1` and `/mcp`
  become available to the `legacy` client for reads.
- The Go version changes from 1.23 to 1.25+ because the MCP SDK requires it. The
  Dockerfile's builder image changes with it; the runtime image doesn't.
