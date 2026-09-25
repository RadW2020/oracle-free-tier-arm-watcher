# Agent-readiness review

Phase 1 audit, 2026-09-25, at commit `95eb62b`. Scope: the whole repository — the
Go service, the Checkly project, the deploy pipeline and every document. Baseline:
`go vet ./...` is clean and `go test ./...` passes.

## Product summary

One person runs about five personal products (Shogunito, Ciaobox, AIDRA, Strong
Core and this watcher) on a single Oracle Cloud *Always Free* ARM VM in Madrid.
The VM uses the full free allowance: 4 OCPUs, 24 GB of RAM and 200 GB of disk. This
repository is how that person keeps the setup free and working. It answers two
questions:

1. **"Am I about to be billed?"** A small Go service reads the tenancy through the
   OCI SDK and compares usage with the Always Free limits. Its key design decision
   (`assessQuotas`, `main.go:336`) separates two kinds of quota:
   - **Allocated quota** — OCPUs, RAM, block storage, IPs and Autonomous DBs. Being
     at 100 % is the goal of a well-used free tier, so it is reported but never
     raises the status.
   - **Accruing quota** — object storage, DB storage and egress. It fills up on its
     own and ends in an invoice, so it drives the status: `OK`, `ATTENTION`,
     `WARNING` or `CRITICAL`.
2. **"Why is everything slow?"** Added after the 17/09/2026 incident
   (`POSTMORTEM-2026-09-17.md`). The service also reads host saturation from OCI
   Monitoring: CPU, real VNIC ingress and egress, and packets dropped by OCI's
   ingress shaper. These signals never change the quota status: a dropped packet
   costs nothing.

The repo also holds the **entire Checkly account as code** (`checkly/`): 18 checks
across the five products, 3 groups, one email channel, a dashboard and a status
page. They are deployed with the Checkly CLI.

There is no web UI of its own. Humans use the product through curl, Grafana Cloud
dashboards (fed by `/metrics` through Alloy), Checkly emails and dashboards, the
OCI console, and the `oci` CLI during incidents.

```
OCI APIs (Compute, Block/Object Storage, LB, DB, VCN, Monitoring)
        │  List*/Get*/SummarizeMetricsData — read-only
        ▼
Go watcher (main.go, oci.go, metrics.go) — one package, no persistence
   ├── GET /usage /status /limits /health   → Checkly API checks, humans
   └── GET /metrics (Prometheus, refreshed every 15 min by a goroutine)
                     → Alloy → Grafana Cloud
Deploy: push to main → GitHub Actions → GHCR (arm64) → Coolify webhook
```

## Existing agent-accessible capabilities

| Interface | What it gives an agent | Notes |
|---|---|---|
| `GET /usage` | Full snapshot: every quota section, saturation, `status`, `warnings`, the limits | Makes 13 + N live OCI calls per request (N = buckets) |
| `GET /status` | `status`, `maxUsagePercentage`, `allocationPercentage`, `warnings` | Same cost as `/usage` |
| `GET /limits` | The hard-coded Always Free limits | Static |
| `GET /health` | Liveness | Says nothing about OCI reachability or data freshness |
| `GET /metrics` | Prometheus gauges, including `oci_watcher_last_update_timestamp` | Public; the only view of the background worker's data |
| Checkly CLI (`checkly/`) | `validate`, `test --grep`, `deploy --preview`, `deploy` | Already usable by a coding agent and has a dry-run step |
| `oci` CLI + `.env` | Anything the OCI user is allowed to do | Used in postmortem §6; see gap 11 |
| Grafana Cloud / Loki HTTP APIs | Metric history, AIDRA logs | Separate products and tokens, outside this repo |

The API responses already include explicit `timestamp`, `configured` and
`sampleAgeSeconds` fields. The Checkly checks assert on `$.maxUsagePercentage`,
`$.configured` and `$.usage.bandwidth.percentage`. That makes the current JSON a
public contract that must stay backward compatible. The 18/09 `omitempty`
regression (`main.go:221-226`) shows what happens when it breaks.

## Human-only capabilities

These workflows need a UI, a privileged credential or manual arithmetic today:

1. **Investigating a past time window** — the postmortem's core workflow. The
   watcher only reports "now". Answering "what happened between 17:00 and 20:00"
   meant using the `oci` CLI with the tenancy signing key and hand-written MQL
   queries, and knowing three things only documented in prose:
   - Buckets are labelled with the *end* of their interval.
   - Real traffic lives in `oci_vcn`; `oci_computeagent` also counts Docker
     interfaces.
   - Monitoring publishes minutes late.
2. **Quota history and trends** only exist in Grafana Cloud (PromQL in its UI).
3. **Projecting egress to month end** is mental arithmetic. `BANDWIDTH_TODO.md`
   already asked for `projectedGB` / `daysRemaining`.
4. **Capacity questions** ("can I host one more service?") are worked out by hand
   from `/usage`. Memory utilisation is not collected at all.
5. **Correlating a Checkly alert with host saturation** means checking Checkly
   emails and the dashboard against Grafana.
6. **The Checkly run budget** (6,570 / 10,000 API runs per month) is computed by
   hand in `checkly/README.md` and goes stale with every frequency change.
7. **Budget-alert state** (the $1 OCI budget) is only visible in the OCI console.

The status page is updated by hand. That is out of scope on purpose: the Hobby
plan cannot automate it, and the owner decided not to invest in it.

## Agent-readiness gaps

Ordered by how badly they would mislead or block an autonomous client.

### A. The agent would reach wrong conclusions

1. **An unknown value is reported as zero.** When an OCI section fails, the
   response sets its `error` string and leaves the percentages at 0.
   `assessQuotas` then counts them as 0, so a failed bandwidth query still gives
   `status: "OK"`. Two variants are worse:
   - `getPublicIPsUsage` drops errors entirely (`oci.go:144-167`); `UsageMetric`
     has no error field.
   - Failed buckets use a `sizeGB: -1` sentinel (`oci.go:341`).

   Asked "am I at risk?", an agent would answer "no" when the honest answer is
   "unknown". This is the most important gap: *"does not invent missing
   information"* is impossible when the API does it first.
2. **Only one compartment is counted, but the limits are tenancy-wide.**
   `getCompartmentID` (`oci.go:50`) reads one compartment, the root by default, and
   not its subtree. Resources in child compartments are invisible, and the
   documentation doesn't say so.
3. **Only `RUNNING` instances are counted** (`oci.go:184`). Whether `STOPPED`
   instances still count against the A1 limit is not documented, so the number
   cannot be interpreted with confidence.
4. **`/status` and `/usage` answer the same question differently.** The
   throttle-drops warning is only added in `usageHandler` (`main.go:433-437`), not
   in `statusHandler` (`main.go:479-487`).
5. **Several names need a code comment to decode:**
   - `maxUsagePercentage` is the maximum of the *accruing* quotas only.
   - `status` never includes saturation.
   - Percentages are truncated to integers, so 0.9 % becomes 0.
   - Units are mixed: `egressGB` next to `limitTB`, and MB/min for rates.
6. **The documentation contradicts the code:**
   - `README.md:147` and `QUICKSTART.md:107` define the status as "overall usage
     below 60 %". That has been wrong since `d9cae80`.
   - The README's `/usage` example has no `allocationPercentage`, `bandwidth` or
     `saturation`.
   - `CHANGELOG.md:86` says 5 goroutines; there are 8.
   - `checkly/SECURITY.md:41` points to a file that doesn't exist.
   - `GRAFANA_GUIDE.md:62` uses `auth {}`, while `config.alloy` uses `basic_auth`.

   An agent that trusts the documentation misreads the status.

### B. Operating it autonomously would be unsafe or fragile

7. **Every read triggers a live fan-out to OCI.** Each request makes 13 + N calls,
   even though the background worker already fetches everything every 15 minutes
   (`metrics.go:247-276`) and then throws the data away except for the gauges.
   There is no rate limit. An agent polling in a loop multiplies the load on OCI.
   The Checkly monitor was slowed to 3 h for exactly this reason
   (`free-tier-monitor.check.ts:29-32`).
8. **Nothing has a timeout.** All 11 OCI call sites use `context.Background()`, and
   `http.ListenAndServe` (`main.go:637`) runs without server timeouts. A hung OCI
   call leaves the client hanging too.
9. **Errors are inconsistent and not actionable:**
   - 405 is plain text (`main.go:285`).
   - 401 is `{"error": "..."}` without a code.
   - `NOT_CONFIGURED` returns HTTP 200 on `/usage` but 503 on `/status`.
   - `ERROR` returns the raw OCI SDK message.

   No error has a stable code or says whether a retry would help.
10. **Authentication is one shared key with weak defaults:**
    - If `API_KEY` is unset, the service allows everything (`main.go:501-506`).
    - The key is compared in non-constant time (`main.go:521`).
    - No client has its own identity, so the logs can't tell Checkly from an agent.
    - `/metrics` is public and exposes bucket names as labels.
11. **Today's investigation path needs too much privilege.** The postmortem
    workflow needs the OCI user's signing key. This repo doesn't document that
    user's IAM policy. The watcher itself only calls `List*`, `Get*` and
    `SummarizeMetricsData`, so it could run on a read-only policy, but nothing
    documents or enforces that.

    Handing an agent the `.env` hands it everything that user can do in the
    tenancy. The Checkly account key has the same problem: it can read `locked`
    variables in clear (`checkly/README.md`).

### C. The agent can't discover what's valid

12. **There is no machine-readable contract.** No OpenAPI, no JSON Schema and no
    version prefix. The status enum values only exist in code.
13. **There is no time dimension.** No endpoint takes a time range, a resolution
    or a metric name. The lag and bucket-labelling rules that make time data
    interpretable only exist in comments and the postmortem.
14. **Important invariants are scattered through Spanish comments.** There is no
    `AGENTS.md`. A coding agent would have to read everything to learn that:
    - The Checkly project's `logicalId` must never change.
    - Deleting a construct deletes the live resource on the next deploy.
    - `checkly test` on the Ciaobox trigger fires a real business POST.
    - Variables with special characters need `{{{VAR}}}`.
    - Groups override their checks' locations and retries.
    - The plan limits (single retry for uptime monitors, one dashboard).

### D. It can't be tested or shown without a real tenancy

15. **OCI access has no seam for tests.** Each `get*Usage` builds its own SDK client
    (`oci.go:147`, `174`, …). Nothing below `assessQuotas` can be tested: the OCI
    mapping, the handlers and the auth have no tests at all.
16. **There is no mode without credentials.** `docker compose up` needs real OCI
    keys and a mounted key file. No local demo is possible, and no deterministic
    eval.
17. **Nobody can see what a client did.** Only auth failures are logged.
    Successful requests have no access log, request ID or duration, and OCI call
    latency and failures are not metrics. "What did the agent do?" has no answer
    today.

## What is already right

- **The domain model is sound.** The allocated/accruing split is exactly the
  distinction an agent needs to answer "should I worry?". It lives in one function
  shared by `/usage`, `/status` and the metrics, so the three cannot disagree on
  quota status.
- **The product is read-only against OCI by construction.** An agent surface can
  inherit that property instead of retrofitting guards.
- **Monitoring changes already have a dry run.** `validate`, `test` and
  `deploy --preview` in `checkly/` is a preview-then-apply flow a coding agent can
  follow.
- **The comments explain *why*.** The postmortem is a worked example of the
  investigation an agent should be able to repeat without the signing key.

## Constraints this sets for the design

- **The most valuable missing capability is time-bounded saturation data**, served
  with the watcher's existing credentials. The postmortem ran that investigation
  by hand.
- **Every agent surface needs a cached snapshot first.** Otherwise each tool call
  is 13 + N OCI calls.
- **The agent surface stays read-only against OCI.** The only mutating workflow in
  the repo (Checkly deploys) belongs to coding agents with the existing preview
  step. It gets documented in `AGENTS.md`, not exposed as a tool.
- **Unknown must be representable.** Every signal needs an explicit availability
  state, and the overall verdict must degrade to "incomplete" instead of "OK".
- **The JSON paths Checkly asserts on stay stable.** New structure is additive or
  versioned.
- **Demos and evals need a fixture data source.** Replaying the 17/09 incident is
  the natural choice.
