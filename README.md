# Oracle Free Tier Watcher

A small Go service that keeps an Oracle Cloud **Always Free** tenancy free and
explains it when it gets slow. It watches one fully allocated ARM VM (4 OCPU /
24 GB / 200 GB) that hosts four other products, and it is operated by **humans
and AI agents as first-class clients of the same API**.

It answers two questions:

- **"Am I about to be billed?"** Usage against the free limits. The service
  separates quota that is *allocated by design* (100 % OCPUs, RAM and disk is
  the goal, not an incident) from quota that *accrues on its own* (object
  storage, DB storage, monthly egress). Only accruing quota can end in an
  invoice, so only it drives the status.
- **"Why is everything slow?"** Host saturation from OCI Monitoring: real VNIC
  traffic, packets dropped by OCI's ingress shaper, CPU and memory. It was added
  after the [17/09/2026 incident](POSTMORTEM-2026-09-17.md), when a satellite
  download saturated the VM while every quota stayed green.

The repo also holds the whole Checkly account as code (`checkly/`): 18 checks
across the hosted products.

## Agent-first architecture

### What an agent can do

An agent connected over MCP (or REST) can answer the operator's real questions
without a console, a shell or the OCI signing key:

| Ask the agent | Tool it uses |
|---|---|
| "How is my free tier doing?" | `get_free_tier_status` → follows its `nextSteps` |
| "Am I going to be charged this month?" | `assess_billing_risk`: thresholds + month-end egress projection |
| "Why did the checks time out at 17:14 on the 17th?" | `get_saturation_timeline`: 1m series, drop intervals, publication lag |
| "What's using my storage? Can I host one more service?" | `get_quota_usage`: per-resource breakdown, name search |
| "Is the watcher broken or is it OCI?" | `get_watcher_diagnostics`: per-source errors, retryability, snapshot age |
| "Get me fresh numbers" | `refresh_usage_snapshot`: the only side effect; scoped, cooldown, idempotent |

### Why the interface exists

The postmortem investigation was done by hand. It meant using the `oci` CLI with
the tenancy's signing key, hand-written MQL and a UTC/CEST conversion, and
knowing three things written down nowhere else:

- Monitoring buckets are labelled with the *end* of the interval.
- `oci_computeagent` also counts Docker's interfaces.
- The data arrives minutes late.

Letting an agent do the same work used to require handing it that key: whatever
the OCI user can do, the agent could do.

Now the watcher, which already holds the credentials, exposes the investigation
as typed, read-only operations. The agent gets a scoped watcher key. The domain
knowledge moved into the schemas, the tool descriptions and the MCP server
instructions.

### Same product, two kinds of client

```
   Human                                   AI agent (Claude Code, Cursor, Codex…)
     │                                        │
 curl · Grafana · Checkly                  MCP (/mcp)  ·  REST (/v1, /openapi.json)
     │                                        │   scoped keys · audit log · rate limits
 legacy /usage /status /metrics               │
     └──────────────┬─────────────────────────┘
                    ▼
        internal/api — 6 operations, one catalog, one error model
                    │
        internal/snapshot — cached reading, guarded refresh ◄── 15-min worker ──► Prometheus
                    │
        internal/freetier — domain rules (quota families, verdicts, projection)
                    │
        internal/source — OCI SDK (read-only)  |  fixture scenarios (demo, tests, evals)
```

REST and MCP are thin adapters over the same `internal/api` operations. A
parity test fails if they ever answer differently. The legacy endpoints that
Checkly asserts on keep their exact JSON, pinned by golden-file contract tests
written before the refactor.

### Design choices that matter to an agent

- **Unknown is a value.** A failed OCI source used to become `0 %` and
  `status: OK`. Now it is `available: false`, and the verdict degrades to
  `UNKNOWN`. A known `CRITICAL` is never hidden by a missing value.
- **Values are discoverable.** Quota IDs, metrics, resolutions and verdicts are
  enums in the schemas, generated from the same Go types for MCP and OpenAPI.
  Unknown query parameters are rejected, so a typo can't silently widen a
  filter.
- **Errors are actionable.** Every error carries a stable `code`, `field`,
  `hint`, `retryable`, `retryAfterSeconds` and `requiredScope`. Example:
  `out_of_range` with the earliest allowed start.
- **Responses suggest the next step.** Each response includes `meta`
  (`dataSource`, `scope`, `observedAt`, `snapshotAgeSeconds`, `complete`,
  `requestId`) and `nextSteps`.
- **Sizes fit a model's context.** Timelines are capped at 720 points; the
  summaries (peaks, drop intervals) come back even with `includePoints=false`.

### Evals

`evals/` holds 11 product tasks as data:

- status on a normal day and with missing data;
- billing risk, safe and at risk;
- the 17/09 incident;
- a window beyond retention;
- capacity;
- an ambiguous bucket name;
- a broken watcher;
- a missing permission;
- a destructive request built on a false premise.

Each case gives the agent a prompt and asks for a small JSON answer contract.
Grading covers the outcome (for example, `cause == ingress_throttle`,
`egressGB is null` when it can't be known, a crossing date within ±1 day) and
the behaviour, read from the **server's audit log**:

- it queried a window covering 15:14–15:20 UTC;
- it made no mutating calls;
- it didn't retry a denied refresh in a loop.

```bash
go run ./evals                                   # scripted: reference must pass, adversarial must fail (CI)
go run ./evals -runner claude-code -model sonnet  # a real agent with only the watcher's tools
```

The scripted runner proves the graders catch regressions. Every case ships
trajectories that are wrong on purpose: blaming CPU, forgetting the timezone,
answering "0" for unknown egress, picking one of two matching buckets. The
graders must reject every one of them. The Claude Code runner uses an empty
working directory, `--tools ""` and `--strict-mcp-config`, so whatever the agent
solves, it solved through the product's interface.
Latest real-agent runs (2026-09-25): **Sonnet 11/11** ($0.58 for the suite) and
**Haiku 10/11** (it read the per-minute drop peak as the burst total). Reports
are in `evals/baselines/` and the analysis is in
[the portfolio review](docs/agent-portfolio-review.md#evals).

### Observability

Every `/v1` and MCP call leaves one audit event with:

- client, transport, MCP session and request ID;
- operation and arguments;
- outcome and error code;
- duration, data source and what it changed.

Events go to the JSON logs, to an in-memory ring (`GET /v1/audit`, scope
`audit`) and to Prometheus (`watcher_requests_total`,
`watcher_request_duration_seconds`). This answers "what did the agent do?",
"why did it fail?" and "what did it modify?". The last answer is always "the
snapshot, or nothing".

### Security model

- **Read-only against OCI by construction.** Only `List*`, `Get*` and
  `SummarizeMetricsData`, and no tool can change the tenancy.
- **Named clients with scopes.** `API_CLIENTS=name:read+refresh:key`, keys
  compared as SHA-256 digests in constant time. The legacy `API_KEY` maps to a
  read-only client, so Checkly needed no change.
- **`/v1` and `/mcp` fail closed.** Without configured clients they reject every
  request. `AUTH_MODE=disabled` is refused unless the data source is a synthetic
  fixture.
- **Guards that protect OCI:**
  - timeline calls are rate-limited per client (20/min);
  - refresh has a cooldown and coalesces concurrent callers;
  - every OCI call has a timeout;
  - raw OCI error text (OCIDs, request IDs) goes to the logs, never to clients.

### Examples

- [Investigate why the checks timed out at 17:14 on 17 September](examples/agent-workflows/01-investigate-incident.md) — read-only investigation
- [Will I be billed this month, and can I host one more service?](examples/agent-workflows/02-billing-and-capacity.md) — multi-step
- [Add a Checkly alert for OCI throttle drops](examples/agent-workflows/03-add-throttle-drops-check.md) — a state-changing workflow behind a preview and a human gate

## Quick start (demo, no OCI account needed)

```bash
docker compose up            # synthetic reconstruction of the 17/09 incident on :8088

curl -H "Authorization: Bearer demo-agent-key" localhost:8088/v1/status
curl localhost:8088/openapi.json

# Connect Claude Code (the repo also ships a .mcp.json with the same settings)
claude mcp add --transport http oci-watcher http://localhost:8088/mcp \
  --header "Authorization: Bearer demo-agent-key"
```

Other scenarios: `FIXTURE_SCENARIO=quiet|egress-at-risk|egress-unavailable|not-configured docker compose up`.
Every scenario is synthetic and says so in `meta.dataSource`.

Cursor and other MCP clients use the same URL and header (Cursor:
`.cursor/mcp.json` with `url` and `headers`). Only the Claude Code path is
tested here.

## Running against a real tenancy

| Variable | Purpose |
|---|---|
| `OCI_TENANCY_ID`, `OCI_USER_ID`, `OCI_FINGERPRINT`, `OCI_PRIVATE_KEY_PATH`, `OCI_REGION` | OCI API signing key (OCI Console → Profile → API Keys). Give that user a read-only policy. |
| `OCI_COMPARTMENT_ID` | Compartment to measure (default: the tenancy root). Child compartments are **not** included. |
| `API_CLIENTS` | `name:scope+scope:key,...` with the scopes `read`, `refresh` and `audit` |
| `API_KEY` | Legacy single key: the client `legacy` with `read` |
| `METRICS_INTERVAL` | Snapshot refresh interval (default `15m`) |
| `REFRESH_COOLDOWN` | Minimum time between externally requested refreshes (default `1m`) |
| `DATA_SOURCE` | `oci` (default) or `fixture` (+ `FIXTURE_SCENARIO`) |

```bash
cp .env.example .env && $EDITOR .env
docker compose --profile oci up oracle-watcher      # or: go run .
```

Production runs on the same VM through Coolify (the Dockerfile). A push to
`main` builds an arm64 image in GitHub Actions, gated by the tests and the
scripted evals, and triggers the deploy. See [QUICKSTART.md](QUICKSTART.md).

### Endpoints

| Endpoint | Auth | Notes |
|---|---|---|
| `GET /v1/status`, `/v1/quotas`, `/v1/billing-risk`, `/v1/saturation`, `/v1/diagnostics` | `read` | Served from the cached snapshot; the timeline reads OCI live |
| `POST /v1/snapshot/refresh` | `refresh` | Re-reads OCI; cooldown and coalescing |
| `GET /v1/audit` | `audit` | What each client did |
| `POST /mcp` | any valid key | Streamable HTTP MCP, stateless |
| `GET /openapi.json` | public | OpenAPI 3.1, generated from the code |
| `GET /usage`, `/status`, `/limits` | legacy key or `read` | Legacy JSON, frozen for Checkly; reads OCI live |
| `GET /health`, `/metrics` | public | Liveness; Prometheus for Alloy → Grafana ([guide](GRAFANA_GUIDE.md)) |

### What the status means

`status` covers **accruing quota only**:

| Status | Accruing quota |
|---|---|
| `OK` | < 60 % |
| `ATTENTION` | 60–80 % |
| `WARNING` | 80–90 % |
| `CRITICAL` | ≥ 90 % |
| `UNKNOWN` (`/v1` only) | Everything known is OK but an accruing quota could not be read |

Egress warns at 50 % (10 TB goes fast when something runs away); the other
accruing quotas warn at 80 %. The allocation of OCPUs, RAM and disk is published
as `allocationPercentage`, and at 100 % that is the point.

### The real safety net

Keep an OCI **budget alert** at $1 with a 1 % actual rule and a forecast rule.
The watcher tells you *before* a quota bills; the budget alert tells you if
anything at all did.

## Development

```bash
go vet ./... && go test -race ./...     # unit, integration, legacy contracts, MCP/REST parity
go run ./evals                          # scripted evals
```

- [AGENTS.md](AGENTS.md): invariants, dangerous operations and conventions, for
  coding agents and for humans.
- [docs/](docs/): the agent-readiness review, the use cases, the plan and a
  critical portfolio review.
- [checkly/README.md](checkly/README.md): the monitoring-as-code half.
- [POSTMORTEM-2026-09-17.md](POSTMORTEM-2026-09-17.md) and
  [CHANGELOG.md](CHANGELOG.md).
