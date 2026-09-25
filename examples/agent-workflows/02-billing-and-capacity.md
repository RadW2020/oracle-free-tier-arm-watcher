# Will I be billed this month, and can I host one more service?

**Kind:** multi-step, two questions answered from three tools · **Use cases:** UC2
and UC5
**Run:** Claude Code (Sonnet), headless, MCP only. Scenario: `egress-at-risk`, in
which something has been serving ~409 GB/day since the 1st. Recorded twice on
2026-09-25, before and after a fix this workflow exposed.

## The request

> Two things. My Oracle bill has always been zero: will it still be zero this
> month? And I want to deploy one more small service that needs 1 OCPU and 4 GB
> of RAM. Can I do that without leaving the free tier?

This scenario is built to trap a shallow check. **The quota status is `OK`**:
egress is at 45.9 %, below its 50 % warning threshold. Only the month-end
projection shows the problem.

## First run: the answer was right, one call was wasted

| # | Call | What came back |
|---|---|---|
| 1 | `get_free_tier_status` | `status: OK`, "Bandwidth 45.9 %" |
| 2 | `assess_billing_risk` | **`AT_RISK`**: egress `projected_over`, "at 408.7 GB/day the free 10240 GB run out on 2026-09-26", 119.7 % projected |
| 3 | `get_quota_usage {quota: arm_ocpus}` | 4 / 4, `coolify-host` |
| 4 | `get_quota_usage {quota: arm_memory}` | 24 / 24 |
| 5 | `refresh_usage_snapshot {reason: "Snapshot is 13 days old…"}` | `refreshed: false`, `reason: cooldown`, `retryAfterSeconds: 37` |

Both conclusions were correct:

- Not a zero bill: egress crosses the limit on 26 September.
- No room for a new instance, since 4/4 OCPUs and 24/24 GB are allocated.
  Deploy the service inside the existing VM, which runs at CPU 5.6 % and memory
  31.5 %.

Call 5 was the interesting one. The scenario runs on a clock frozen at 12
September, and the agent compared `observedAt` with **its own** date (25
September), concluded the data was 13 days old and tried to refresh.
`meta.snapshotAgeSeconds` said 0, and the agent ignored it. The guard did its
job: the refresh was within its cooldown, so it cost nothing, changed nothing
and is visible in the audit log. The agent reported it honestly ("a refresh
attempt just hit cooldown").

**The fix went into the product, not the prompt.** The MCP server instructions
and the schema description of `meta.generatedAt` now say that `generatedAt` is
the watcher's "now" and that ages must come from it and
`snapshotAgeSeconds`, not from the model's own date.

## Second run, after the fix

| # | Call | What came back |
|---|---|---|
| 1 | `assess_billing_risk` | `AT_RISK`, crosses on 2026-09-26 |
| 2 | `get_quota_usage {quota: arm_ocpus}` | 4 / 4 |
| 3 | `get_quota_usage {quota: arm_memory}` | 24 / 24 |

The run took three calls instead of five, made no refresh attempt and cost $0.055
instead of $0.076. The answer (trimmed):

> **No — this month is flagged AT_RISK, and it's egress, not storage** …
> 4,700 GB used of the 10,240 GB free allowance (45.9 %), running at
> ~408.7 GB/day. Projected to hit 12,260.9 GB (120 %) by month end, crossing the
> free limit around 2026‑09‑26.
>
> **No, not as a separate instance.** Your ARM Always Free allowance is 100 %
> allocated already, entirely to `coolify-host` … Deploy the new service on the
> existing VM … or resize it down.
>
> I can't resize or deploy anything myself (this tool is read-only).

## What this shows about the interface

- **One verdict per question.** `assess_billing_risk` exists because
  "status OK" and "you will be billed" are both true here, and an agent reading
  only the status would answer wrong. Eval case `billing-at-risk` has an
  adversarial trajectory that does exactly that, and the graders reject it.
- **"Allocated at 100 %" was not read as a problem.** The quota families come
  from the schema, and the agent went straight to "use the existing VM".
- **The only mutation is safe to try.** A mistaken refresh cost nothing and was
  recorded, and the response told the agent when it could retry.
- **Agent behaviour found a product bug.** The ambiguity about which clock is
  "now" was a flaw in the interface. The fix was a sentence in the interface
  itself.
