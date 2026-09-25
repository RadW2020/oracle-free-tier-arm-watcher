# Agent use cases

These are the tasks an agent (Claude Code, Codex, Cursor or any MCP/HTTP client)
should be able to complete with this product. Each one is a question the operator
already asks today, taken from the postmortem, the TODO documents and the Checkly
README. None were invented for this exercise. Gap numbers (#1–#17) refer to
`docs/agent-readiness-review.md`.

Permissions use three credentials:

| Credential | Grants | Who holds it |
|---|---|---|
| **watcher read key** | Read the watcher API: snapshot, saturation history, limits | Agents, Checkly |
| **watcher refresh** | Also force a fresh OCI read, rate-limited | Agents, only when explicitly scoped |
| **Checkly account key** | Deploy monitoring changes; it can also read `locked` variables | The human operator only |

No use case needs the OCI signing key. Keeping it off agent machines is a design
goal (gap #11).

---

## UC1. Overall state — "How is my Oracle Free Tier doing?"

- **Intent:** a one-glance answer that separates "fine" from "unknown".
- **Required information:**
  - Quota status and its drivers.
  - Allocated vs accruing percentages.
  - Current saturation.
  - Snapshot age.
  - Which data sources failed.
- **Sequence:** read the cached snapshot → if it is stale or incomplete, say so
  and optionally request one refresh → summarise.
- **Expected output:**
  - The status, with the quota that drives it.
  - Allocation, framed as expected ("100 % allocated by design").
  - Any saturation warning.
  - The data's age.
  - An explicit list of unavailable signals.
- **Failure cases:**
  - OCI not configured.
  - Partial OCI failure: must be reported as *unknown*, never as 0 % (gap #1).
  - The background refresh has been failing, so the data is stale.
  - The watcher is unreachable.
- **Permissions:** watcher read key. **Read-only.**

## UC2. Billing risk — "Am I going to be charged anything this month?"

- **Intent:** find out whether an accruing quota will cross its free limit before
  month end.
- **Required information:**
  - Object storage, DB storage and month-to-date egress, each with its limit.
  - Days elapsed and remaining in the month.
  - The warning thresholds: 50 % for egress, 80 % for the others.
  - Whether each value is known.
- **Sequence:** read the snapshot → for each accruing quota, check availability →
  project egress linearly to month end → compare with the thresholds.
- **Expected output:** per quota:
  - Used / limit / %.
  - A projected month-end value and the day it would cross, where that applies.
  - A verdict of `no risk`, `at risk` or `unknown`, with the reason.
- **Failure cases:**
  - The egress query failed.
  - Early in the month, so the projection is not meaningful: say so rather than
    extrapolate from one day.
  - The OCI budget alert is not visible through this product: the agent must say
    it cannot confirm billing state beyond quota.
- **Permissions:** watcher read key. **Read-only.**

## UC3. Past degradation — "Why did the checks time out around 17:14 on 17 September?"

- **Intent:** find the host-level cause of a slowdown the operator was alerted
  about. This is the postmortem workflow, without the signing key.
- **Required information:**
  - A bounded time window in UTC.
  - 1-minute series for VNIC ingress bytes, ingress throttle drops, egress bytes
    and CPU.
  - The data lag and how buckets are labelled.
  - Which namespaces are authoritative (`oci_vcn`, not `oci_computeagent`).
- **Sequence:**
  1. Normalise the user's time to UTC. The operator's times are CEST.
  2. Fetch the saturation series with a margin around the window.
  3. Find intervals where drops are above 0 and when ingress peaks.
  4. Check CPU for a competing explanation.
  5. Optionally fetch the same window on previous days for comparison. The
     postmortem did this for 15/16/17 September.
- **Expected output:**
  - The cause category: *ingress throttling*, *CPU saturation* or *nothing visible
    at host level*.
  - Supporting peaks with timestamps.
  - The minutes the evidence does **not** explain. The postmortem's "loose end" is
    the model to follow: a failure 50 s before the download started stays
    unattributed.
- **Failure cases:**
  - The window is beyond OCI Monitoring retention.
  - The window is too large for the resolution: the server must reject it and
    return the allowed maximum.
  - The timezone is ambiguous.
  - The window includes the last few minutes, which are not published yet.
  - Application logs (AIDRA's Loki) are outside this product: say so, don't guess.
- **Permissions:** watcher read key. **Read-only.**

## UC4. Live pressure — "Everything feels slow right now. Is it the host?"

- **Intent:** tell host saturation apart from a single application's problem while
  it is happening.
- **Required information:** the last hour of saturation data, the snapshot age and
  the drops over the last hour.
- **Sequence:** fetch the saturation series for the last 60 min → check drops and
  ingress trend → report. This is UC3 with `end = now`, but the answer has to
  account for publication lag.
- **Expected output:**
  - Whether the shaper is dropping packets now or recently, and how many.
  - The ingress rate and the CPU.
  - How old the newest point is.
- **Failure cases:**
  - No datapoints yet.
  - The saturation source failed.
  - The newest data is older than ~10 min: warn rather than conclude.
- **Permissions:** watcher read key. **Read-only.**

## UC5. Capacity — "Can I host another service that needs 1 OCPU, 4 GB of RAM and 50 GB of disk?"

- **Intent:** decide whether a new deployment fits without leaving the free tier.
- **Required information:** allocated usage vs limits for OCPUs, RAM, block
  storage and instances, plus current CPU utilisation of the existing VM.
- **Sequence:** read the allocated quotas → compute account-level headroom → if it
  is zero (the normal case), check utilisation inside the existing VM → answer.
- **Expected output:**
  - Account-level headroom per resource.
  - An explicit statement that new capacity must come from inside the existing VM.
  - CPU utilisation as evidence.
  - Memory utilisation of the VM (`memory_percent`). It was not collected at
    audit time; it was added so that RAM fit can be judged.
- **Failure cases:**
  - Compute data unavailable.
  - Resources in child compartments are not counted (gap #2): the agent must
    mention that scope.
  - The request exceeds the free limits outright.
- **Permissions:** watcher read key. **Read-only.**

## UC6. What is consuming a quota — "What's using my object storage?"

- **Intent:** find the specific resource behind a quota number.
- **Required information:**
  - Per-bucket sizes.
  - Boot and block volume counts and sizes.
  - Load balancers with shape and state.
  - Autonomous DBs.
- **Sequence:** read the snapshot → sort the relevant section by size → report the
  top contributors against the limit.
- **Expected output:** a ranked list of resources with sizes and their share of
  the limit. Resources whose size couldn't be read are flagged, never counted as
  0 GB.
- **Failure cases:**
  - A bucket's size is unreadable (the `-1` sentinel today, gap #1).
  - The quota is in another compartment.
  - An ambiguous name matches several resources: list all matches, don't choose
    one.
- **Permissions:** watcher read key. **Read-only.**

## UC7. Watcher self-diagnosis — "Checkly says the Free Tier Monitor failed. Is the watcher broken or is OCI?"

- **Intent:** decide whether to trust the monitoring before acting on it.
- **Required information:**
  - When the last refresh succeeded.
  - Errors per data source.
  - Whether OCI credentials are configured.
  - Whether authentication is enforced.
  - The service version.
- **Sequence:** call health or diagnostics → read the per-source status → if one
  source fails, classify the error (auth, permission, network or throttling) →
  recommend the next step.
- **Expected output:**
  - Healthy, degraded (with the named sources) or down.
  - The error category, and whether a retry makes sense.
  - The age of the last good data.
- **Failure cases:**
  - The watcher is unreachable: that's the answer, stated plainly.
  - Missing credentials.
  - OCI throttling (HTTP 429 from OCI).
  - Clock skew breaking request signing.
- **Permissions:** watcher read key. **Read-only.**

## UC8. Add or change monitoring — "Monitor the new service at `https://x.uliber.com/health` every hour."

This is a **coding-agent** workflow over the repository and the Checkly CLI. The
product already has a preview step, so it gets no new tool. Its difficulty is the
invariants, which is why it belongs in `AGENTS.md`.

- **Intent:** add, retune or remove a check without breaking the account.
- **Required information:**
  - The existing groups.
  - Location and retry rules: groups override their checks, and uptime monitors
    allow only a single retry.
  - The plan limits and the current run budget.
  - Whether the new check needs body assertions, which decides `ApiCheck` vs
    `UrlMonitor`.
- **Sequence:**
  1. Add `src/<area>/<name>.check.ts`.
  2. Compute the change in API runs per month (30 min ≈ 1,460, 1 h ≈ 730,
     3 h ≈ 243) and confirm the total stays under 10,000.
  3. Run `npx checkly validate`.
  4. Run `npx checkly test --grep "<name>"`.
  5. Run `npx checkly deploy --preview` and check that the diff contains only the
     intended `Create`/`Update`.
  6. Hand off to the human for `npx checkly deploy`.
- **Expected output:** the new construct, the budget delta and a preview diff with
  no unrelated changes. Nothing deployed without approval.
- **Failure cases:**
  - Renaming the project `logicalId`: forbidden, it duplicates everything.
  - Deleting a construct: forbidden unless asked, it deletes the live check and
    its history.
  - Running `checkly test` on `ciaobox weekly-close trigger`: forbidden, it fires
    a real business action.
  - A retry strategy rejected by the plan.
  - A missing account variable.
  - A secret with `=` or `&` that needs `{{{VAR}}}`.
- **Permissions:** repository write. The Checkly key and the final `deploy` stay
  with the human. **Mutating**, gated by preview and human approval.

---

## Deliberately not agent use cases

- **Changing OCI resources** (stop or terminate instances, delete buckets). The
  product is a watcher. Giving agents destructive cloud power adds risk without
  product value. The operator uses the OCI console.
- **Updating the status page.** The Hobby plan cannot automate it, and the owner
  chose not to invest in it.
- **Reading application logs** (AIDRA's Loki). That belongs to another product
  with its own credentials. UC3 must state the boundary instead of crossing it.
- **Automatic incident response.** Nothing in the product justifies an autonomous
  loop that acts on alerts.
