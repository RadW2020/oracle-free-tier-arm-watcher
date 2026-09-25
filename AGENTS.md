# AGENTS.md

This repo is the monitoring layer for one Oracle Cloud **Always Free** tenancy:
a single ARM VM (4 OCPU / 24 GB / 200 GB, Madrid) that hosts the owner's other
products (Shogunito, Ciaobox, AIDRA, Strong Core). It has two halves:

- **The watcher** (Go, repo root + `internal/`). It reads the tenancy through the
  OCI SDK, read-only, and answers two questions: *will I be billed?* (quota) and
  *why is everything slow?* (saturation). It serves the legacy endpoints
  (`/usage`, `/status`, …), an agent API (`/v1/*`, `/openapi.json`) and an MCP
  server (`/mcp`).
- **`checkly/`**: the whole Checkly account as code (18 checks, a dashboard and a
  status page).

You will meet this repo either **operating the product** (through MCP or REST) or
**changing it** (Go code or Checkly config). Both are covered below.

## Operating the product

```bash
docker compose up        # demo: synthetic 17/09 incident, no OCI credentials needed
claude mcp add --transport http oci-watcher http://localhost:8088/mcp \
  --header "Authorization: Bearer demo-agent-key"
```

There are six tools: `get_free_tier_status` (start here), `get_quota_usage`,
`assess_billing_risk`, `get_saturation_timeline`, `get_watcher_diagnostics` and
`refresh_usage_snapshot`. The REST equivalents live under `/v1`. The server's
MCP `instructions` and the tool descriptions contain the interpretation rules;
the short version:

- **Allocated vs accruing quota.**
  - Allocated quota (OCPUs, RAM, block storage, IPs, DBs) sits at 100 % *on
    purpose* and never raises the status.
  - Accruing quota (object storage, DB storage, monthly egress) grows on its own
    and gets billed over the limit. Only accruing quota drives `status`.
  - Egress warns at 50 %; the other accruing quotas warn at 80 %.
- **Unknown ≠ 0.** `UNKNOWN`, `available: false`, `sizeKnown: false` and
  `complete: false` mean the value could not be read. Never report it as zero.
- **Times.**
  - All times are UTC; the operator speaks in Europe/Madrid (CEST = UTC+2).
  - Monitoring points are labelled with the **end** of their bucket.
  - OCI publishes a few minutes late.
- **Real traffic lives in `oci_vcn`.** `oci_computeagent` network bytes also count
  the Docker interfaces (~50 GB/day against 8.5 GB/day of real egress).
- **Throttle drops.** `VnicIngressDropsThrottle > 0` means OCI is discarding inbound
  packets *for every service on the host*. That is the 17/09 incident; see
  `POSTMORTEM-2026-09-17.md`.
- **Fixture data.** `meta.dataSource: fixture` is a synthetic scenario. Never
  quote it as account data.

Credentials are named clients in `API_CLIENTS=name:scope+scope:key,...`, with
the scopes `read`, `refresh` and `audit`. The legacy `API_KEY` is the client
`legacy` with `read`. An agent needs `read`, plus `refresh` only if it should be
able to force a re-read. **Agents never get the OCI signing key**: the watcher
holds it and does the calls. `GET /v1/audit` (scope `audit`) shows what each
client did.

## Invariants — do not break these

1. **The legacy JSON is a contract.** Checkly asserts `$.maxUsagePercentage`,
   `$.configured` and `$.usage.bandwidth.percentage`.
   - Changes to `/usage` and `/status` must be additive only.
   - `internal/httpapi/contract_test.go` compares against golden files.
   - Regenerate them (`go test ./internal/httpapi -run Contract -update`) only
     for additive changes, and review the diff.
   - `maxUsagePercentage` must never get `omitempty`: a 0 disappeared and broke
     Checkly on 18/09.
2. **Read-only against OCI.** `internal/source/oci.go` may only call `List*`,
   `Get*` and `SummarizeMetricsData`. No tool may mutate the tenancy. Don't add
   one "for convenience".
3. **Unknown stays unknown.** A failed source must end up in `Reading.Sources` as
   unavailable.
   - Never let a zero value stand in for a failure.
   - Legacy `status` deliberately ignores missing data (contract); the `/v1`
     verdict (`Assess(...).Verdict`) accounts for it.
4. **One place per rule.**
   - Quota thresholds and families: `internal/freetier` (`Quotas`, `Assess`).
   - Percentages: `freetier.Finalize`. Sources measure; they don't compute
     percentages.
5. **REST and MCP are one API.** To add an operation:
   - an entry in `api.Operations` (`internal/api/catalog.go`);
   - a `Service` method that goes through `s.call` (scope check + audit event);
   - a `dispatch` case in `internal/httpapi/v1.go`;
   - a `register` line in `internal/mcpserver/server.go`.

   `TestRESTAndMCPAgree` and the OpenAPI coverage test fail if they diverge.
6. **Every agent-facing call is audited.** Don't add a path around `Service.call`.

## Dangerous operations

- **A push or merge to `main` deploys to production** (GitHub Actions → GHCR →
  Coolify webhook). Work on a branch and let the owner merge.
- **`npx checkly deploy` changes the live account.**
  - Deleting a construct deletes the check and its history; there is no safety net.
  - Never change `logicalId: 'shogunito-project'` in `checkly/checkly.config.ts`:
    it duplicates every resource.
  - Always run `npx checkly deploy --preview` and show the diff to the owner
    before deploying.
- **`npx checkly test` on `ciaobox weekly-close trigger`** POSTs to
  `/api/cron/weekly-close` and fires a real business action. Exclude it from
  `--grep`.
- **`.env` holds live secrets:**
  - the OCI signing key path;
  - `CHECKLY_API_KEY`, which can read `locked` variables in clear;
  - `COOLIFY_ROOT_API_TOKEN`.

  Never print, log or commit them. Checkly secrets are referenced as `{{VAR}}`,
  or as `{{{VAR}}}` when the value contains `=` or `&` (Handlebars escapes HTML).
- **`refresh_usage_snapshot`** costs 13 + N OCI API calls (N = buckets). The
  cooldown protects it; don't loop on it.

## Checkly constraints (Hobby plan)

- **Budget:** 10,000 API-check runs per month, currently ~6,570. A check costs
  about 1,460 runs/month at 30 min, 730 at 1 h and 243 at 3 h. `UrlMonitor`s are
  billed per unit (5 of 10 used), not per run.
- **Groups override** their checks' `locations` and `retryStrategy`. Groups that
  contain uptime monitors must use `monitorRetry` (single retry only).
- **Plan limits:**
  - One dashboard (already used).
  - No `triggerIncident`: the status page is updated by hand and is deliberately
    not maintained.
- Details: `checkly/README.md`.

## Commands

```bash
go vet ./... && go test -race ./...        # unit, integration, legacy contracts, MCP/REST parity
go run ./evals                             # scripted evals (deterministic, CI)
go run ./evals -runner claude-code -model sonnet   # real agent, ~$0.10-0.20 per case
DATA_SOURCE=fixture FIXTURE_SCENARIO=quiet API_CLIENTS=me:read+refresh+audit:k go run .
cd checkly && set -a && . ../.env && set +a && npx checkly validate
```

The scenarios are `quiet`, `incident-2026-09-17`, `egress-at-risk`,
`egress-unavailable` and `not-configured` (`internal/source/scenarios/`).

## Where things are

| Path | What |
|---|---|
| `internal/freetier` | Domain rules: limits, quota catalog, `Assess`, egress projection. Pure. |
| `internal/source` | `Source` interface; `oci.go` (real, read-only); `fixture.go` plus embedded scenarios |
| `internal/snapshot` | Cached reading; refresh with cooldown and coalescing. The 15-minute worker writes it. |
| `internal/api` | The operations, DTOs (their field descriptions become the JSON schemas agents see), error codes |
| `internal/httpapi` | Legacy handlers, `/v1`, generated `/openapi.json`, legacy contract tests |
| `internal/mcpserver` | MCP tools over `internal/api`, server instructions, `/mcp` handler |
| `internal/audit` | Named clients and scopes, audit events (log, ring buffer, Prometheus) |
| `internal/app` | Wiring shared by `main`, the integration tests and the evals |
| `evals/` | Eval harness; `cases/*.yaml` are the product tasks |
| `docs/agent-*.md` | Readiness review, use cases, plan, portfolio review |

## Conventions

- **Code comments are in Spanish** and explain *why* (incidents, trade-offs); keep
  that density. Agent-facing strings (tool descriptions, errors, warnings,
  schema descriptions) are in English.
- **Commit messages are in Spanish**, with a `type(scope): ` prefix (`feat`,
  `fix`, `docs`; scopes like `status`, `metrics`, `checkly`). Write them in the
  imperative, saying what changes for the user.
- **Tests pin behaviour that came out of incidents.** When fixing one, add the
  test that would have caught it, with a comment saying which incident.

## Known edge cases

- **Only one compartment is measured**, not its children, while the free limits
  are tenancy-wide. `meta.scope` says so.
- **Only `RUNNING` instances are counted.** Whether `STOPPED` A1 instances count
  against the limit is unverified.
- **A bucket whose size can't be read** has `sizeGB: -1` in the legacy JSON and
  `sizeKnown: false` everywhere.
- **Memory utilisation needs the OCI compute agent plugin.** Signals without data
  are listed in `saturation.missingSignals`.
- **With neither `API_KEY` nor `API_CLIENTS` set:**
  - the legacy endpoints are open (historical behaviour);
  - `/v1` and `/mcp` reject everything.
- **Suggested OCI policy for the watcher's user:** read-only, e.g.
  `Allow group oci-watcher to read all-resources in tenancy`. This hasn't been
  verified against the live tenancy; the owner has to confirm it.
