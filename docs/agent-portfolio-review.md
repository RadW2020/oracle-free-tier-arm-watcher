# Portfolio review: "built things agents actually use"

Phase 5, 2026-09-25, branch `agent-first`. This is a critical read of the result,
written from the chair of a hiring manager for a Product Engineer role where
"built things agents actually use" is a stated qualification. Praise is kept to
what the evidence supports.

## Evidence first

### Evals

| Run | Result | Cost | Notes |
|---|---|---|---|
| Scripted (CI) | 31/31 | $0 | 11 references pass and 20 adversarial trajectories are all rejected |
| Claude Code · Sonnet, first run | 10/11 | ~$0.60 | `billing-at-risk` failed: the agent refused to treat fixture data as "my bill" (see below) |
| Claude Code · Sonnet, final | **11/11** | $0.58 | 24 tool calls across 11 tasks; median of 2 calls per task |
| Claude Code · Haiku, final | **10/11** | $0.24 | `incident-17-09`: reported the per-minute peak (13,500) as the burst total (81,000) |

Reports are in `evals/baselines/`. The agent had only the watcher's MCP tools:
no shell, no files, no web, and an empty working directory.

What the runs showed:

1. **The first real failure was the eval's fault, not the product's.** The watcher
   labels demo data `dataSource: fixture`, and its instructions say never to
   present it as account data. Asked "is my bill still zero?", Sonnet refused to
   extrapolate from demo data and answered `UNKNOWN`: the label worked as
   designed. The fix went into the harness, which now states the evaluation
   framing. Removing the label would have fixed the eval by breaking the product.
2. **Real use found a real interface bug.** In `examples/…/02`, the agent
   computed the snapshot's age from its own date instead of the watcher's frozen
   clock and attempted a refresh. The cooldown made that harmless. The fix was a
   sentence in the MCP instructions and in the `meta.generatedAt` schema, and
   the rerun needed 3 calls instead of 5 and made no refresh attempt.
3. **The suite discriminates between models.** Haiku confused `peakPerBucket`
   with `totalDrops`. That is the kind of regression the suite exists to catch,
   whether it comes from a model or from a field rename.
4. **An actionable error drove recovery with no scripting.** Example 01: the
   agent asked for too many points, got `invalid_argument` with a hint, and
   switched to `includePoints: false` on the next call.

### Code

The work is about 5.9k lines of Go outside tests plus 1.5k lines of tests, across
7 internal packages. That is more than the plan's "~2.5k" estimate. Much of it is
schema descriptions written for agents and why-comments in the house style, but
it is still a lot of surface for six operations. See Weak spots below.

## Scores

Scale: **Strong** means it would survive a skeptical interview. **Adequate**
means it's there, with known gaps. **Weak** means it needs work before being
claimed.

### Agent usability — Strong

A real agent completes 11/11 product tasks with a median of 2 calls. It recovers
from errors on its own, marks the edge of what it can see, and declines
destructive requests.

Weak spots:

- **Only Claude models were tested.** The Cursor snippet in the README is
  untested, and the harness has no Codex runner.
- **The timeline cap pushes investigations into 3–4 calls.** The agent learns to
  ask for summaries first; a more natural default would be summary-only.
- **Improvements:**
  - Make `includePoints` default to `false` when the window exceeds the cap,
    and say so in the response instead of erroring.
  - Add a Codex runner, the harness's `Runner` interface is ready for it, and
    publish cross-model results.

### API/tool design — Strong

Six domain operations with one catalog, one error model and one schema source
for MCP and OpenAPI. Enums are discoverable, unknown parameters are rejected,
and responses carry `meta`, `nextSteps` and provenance (namespace and query for
every series).

Weak spots:

- **Responses are verbose.** `meta.scope` and the quota descriptions are
  repeated on every call, and `get_quota_usage` returns every description every
  time. That is fine for six tools and adds up in long sessions.
- **Tool errors reach MCP as JSON text** (`isError` + text), not as
  `structuredContent`. Errors raised by the SDK's own schema validation (for
  example, a bad enum) bypass the error envelope and the audit log.
- **Improvements:**
  - Add a `verbose=false` default that drops the static text.
  - Validate enums in `internal/api` before the SDK does, so every rejection is
    an envelope and an audit event.

### Product usefulness — Adequate

The tasks are the operator's real questions, taken from the postmortem and the
TODO files. The throttle-drops alert (postmortem action 3) is now affordable and
written, but **not deployed**.

Weak spots:

- **The measurement scope is narrow.** One compartment only, `RUNNING` instances
  only, and linear egress projection.
- **There is no access to actual billing** (the OCI Usage API), so "will I be
  billed" is inferred from quota, not read from the invoice.
- **The product has one real user.**
- **Improvements, in order:**
  1. Scan compartment subtrees; the limits are tenancy-wide.
  2. Read the OCI Usage API (cost to date) and the budget-alert state, so
     `assess_billing_risk` stops listing them under `notCovered`.
  3. Deploy the drops check after the AIDRA download cap ships.

### Documentation — Strong

`AGENTS.md` is short and holds what code can't say: the invariants, the
dangerous operations and the Checkly traps. The three examples come from real
runs, including the one that exposed a bug.

Weak spots:

- **Languages are mixed.** The code comments and older docs are in Spanish and
  the portfolio docs in English. That is honest to the repo's history, and it
  still costs a non-Spanish reader.
- **The README rewrite dropped the Spanish operational walkthrough**; it now
  lives only in `QUICKSTART.md`.
- **Improvement:** add a one-paragraph English summary at the top of each
  Spanish doc (postmortem, Checkly README).

### Evals — Adequate, close to Strong

Outcome and behaviour graders run against the server's own audit log, and
adversarial trajectories prove the graders catch regressions. The scripted
suite runs in CI, and there are real-agent baselines for two models.

Weak spots:

- **One run per model**: no variance, no pass@k. 11 cases is a small suite.
- **Free text is not graded.** The graders can't judge whether "unexplained"
  mentions the 15:13 failure, for example.
- **The answer contract makes every agent produce the key fields**, which is
  easier than a free-form conversation.
- **Real-agent evals are not in CI** (cost, and they need a logged-in `claude`).
- **Improvements:**
  - Run each case 3–5 times and report pass rates.
  - Add a nightly real-agent job with a cost cap that diffs against
    `evals/baselines/`.
  - Add 2–3 free-form cases graded only on trajectory.

### Safety — Strong for the scope

The interface is read-only against OCI by construction. Clients are named and
scoped, keys are compared in constant time and new surfaces fail closed. The
guards protect OCI (rate limit, cooldown, coalescing, timeouts), and raw
upstream errors are kept away from clients. Agents never hold the signing key,
and deploys and Checkly changes stay behind a human.

Weak spots:

- **Keys are static values in env vars**, with no expiry and no rotation tooling.
- **The legacy endpoints stay open when no key is set** (kept for
  compatibility).
- **`/metrics` is public** and leaks bucket names.
- **The audit ring is in memory** and lost on restart; the logs persist it.
- **The read-only IAM policy is recommended, not verified.**
- **Improvements:**
  - Verify and document the IAM policy against the tenancy.
  - Close the legacy endpoints when no key is set (production already has one).
  - Bind `/metrics` to localhost or behind a key.
  - Add an `expires` field to `API_CLIENTS` entries.

### Observability — Adequate

Every agent call leaves an audit event (client, operation, arguments, outcome,
error code, duration, what it changed) in the logs, in `/v1/audit` and in
Prometheus. Data completeness and snapshot age are gauges.

Weak spots:

- **No per-OCI-call latency or error metrics**: only full reads are counted.
- **No committed Grafana panels** for the new metrics.
- **MCP runs stateless**, so `session` is always empty and a multi-call agent
  session can only be correlated by client and time.
- **Improvements:**
  - Add `watcher_oci_call_duration_seconds{source}`.
  - Add a small dashboard JSON.
  - Accept an `X-Agent-Session` header and record it.

### Developer experience — Strong

`docker compose up` gives a working demo with no cloud account, and `.mcp.json`
connects Claude Code in one step. Tests, contracts and scripted evals take
seconds and gate the deploy.

Weak spots:

- **`go run ./evals` must run from the repository root** (the case paths are
  relative).
- **The real-agent runner is Claude Code only** and costs money.
- **The demo keys are fixed strings.** That is fine for synthetic data, and it
  is also a pattern someone could copy.
- **Improvement:** add a Makefile or `just` recipes (`demo`, `test`,
  `evals-real`) and resolve case paths from the module root.

### Demo quality — Adequate

The scenarios are coherent: saturation values are computed from the same
series the timeline serves, so `/usage` and the timeline can't disagree. The
incident reconstruction matches the postmortem's figures, and every scenario is
labelled synthetic.

Weak spots:

- **Everything is synthetic.** The later re-download drops in the incident
  scenario are assumptions, and are labelled as such.
- **There is no recording or hosted demo.** A reviewer has to run it.
- **Improvements:**
  - Record a 90-second terminal cast of example 01.
  - Capture one real (redacted) scenario with a `record-fixture` command (P2 in
    the plan).

## Deviations from the plan

- **Done beyond P0:** OpenAPI generated from the types, `/v1/audit`,
  `memory_percent`, instance and volume listings, the CI gate, the documentation
  drift fixes and `.mcp.json`. This means UC5 can now judge RAM fit, which the
  use-case document originally ruled out.
- **Not done (P1):** a cache for past timeline windows. Every timeline call goes
  to OCI, bounded by the rate limit.
- **Changed:**
  - Refresh reasons are `refreshed | coalesced | cooldown` (not `in_flight`).
  - MCP errors are JSON text, not `structuredContent`.
  - MCP runs stateless, so there are no session IDs.
  - Legacy endpoints now also accept `Authorization: Bearer` and any `read`
    client (additive).
  - The eval harness adds one framing sentence (see Evidence 1).
- **Written but not deployed** (owner's call):
  `checkly/src/oci/ingress-throttle-drops.check.ts`. **The next `checkly deploy`
  will create it**, and `checkly/README.md` says so.

## Verdict

The repo shows the thing the qualification asks about: a product whose
operations an autonomous agent can discover, call, recover from and be held
accountable for, with evals that would catch it getting worse. The strongest
evidence is not the MCP server. It is these three things:

- the unknown-is-not-zero fix;
- the eval failure that was correctly attributed to the harness;
- the interface bug found by watching a real agent.

What would make it convincing to a skeptic:

1. Cross-model and repeated eval runs.
2. One real (non-synthetic) scenario.
3. The OCI Usage API, so billing questions stop being inferred from quota.
