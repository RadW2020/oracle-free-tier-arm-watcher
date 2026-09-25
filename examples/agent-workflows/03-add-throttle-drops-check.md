# Add a Checkly alert for OCI throttle drops

**Kind:** state-changing, behind a preview and a human gate · **Use case:** UC8
**Run:** done by the coding agent (Claude Code, Opus) that built the agent
interface, in the same session, working on the repository. This workflow
changes a production monitoring account, so it deliberately stops before
anything that touches the account.

## The request

> The next time OCI starts dropping packets I want the email to say so, not just
> "Shogunito timed out". Add a check for it.

This is action 3 of `POSTMORTEM-2026-09-17.md`, still pending.

## What the agent did

1. **Read the constraints first.** They come from `AGENTS.md` and
   `checkly/README.md`:
   - a deploy is live;
   - deleting a construct deletes the check;
   - the project's `logicalId` must never change;
   - the budget is 10,000 API runs per month (~6,570 used);
   - groups override locations and retries;
   - `checkly test` must never run on the Ciaobox trigger.
2. **Chose the data source for cost, not convenience.** The legacy `/usage`
   already exposes `ingressThrottleDropsLastHour`, but every request makes 13 + N
   live OCI calls. That is why the existing Free Tier Monitor runs only every
   3 h. `/v1/status` serves the watcher's cached snapshot and makes **zero** OCI
   calls, so the new check can run hourly for free on the OCI side. It uses the
   existing `ORACLE_MONITOR_API_KEY`: the legacy key is a `read` client on `/v1`.
3. **Checked the coverage arithmetic.** Each snapshot covers the previous hour
   and is at most 15 minutes old, so hourly runs overlap with no gap. Every
   6-minute burst is seen.
4. **Wrote one construct:** `checkly/src/oci/ingress-throttle-drops.check.ts`.
   - It follows the house pattern: shared `LOCATIONS`, `crossRegionRetry`,
     `standardEscalation`, the email channel, and a comment on why it exists.
   - Assertions: status 200, `$.saturation.available == true`, so an unreadable
     signal can't pass as zero drops, and
     `$.saturation.ingressThrottleDropsLastHour < 1`.
5. **Recomputed the budget.** 730 runs/month at 1 h: 6,570 → ~7,300 of 10,000.
6. **Verified offline.** `npx tsc --noEmit -p checkly` passed.
7. **Stopped.** `checkly validate` and `checkly deploy --preview` both call the
   Checkly API with the account key, which can also read `locked` secrets. That
   step belongs to the owner.

## The hand-off

Deploy order matters, and the agent spelled it out:

1. Merge and deploy the watcher branch first. Before that, `/v1/status` returns
   404 and the check would fail on its first run.
2. `cd checkly && set -a && . ../.env && set +a && npx checkly validate`
3. `npx checkly deploy --preview`. **Expect exactly one `Create`**
   (`oci-ingress-throttle-drops`) and no `Update` or `Delete` of anything else.
4. `npx checkly deploy`

Known consequence: until AIDRA's 20 Mbps download cap is in production, the
check will fire on every scene download. That is the signal the postmortem asked
for, but it will send emails, and the owner should know that before deploying.

## What this shows about the interface

- **Mutations go through the tool that already has a preview.** There is no
  "deploy_check" MCP tool. Checkly's CLI already has
  `validate → test → deploy --preview → deploy`; wrapping it would hide the
  diff the human must see, and would hand an agent a key that can read secrets.
- **The new API changed what monitoring can afford.** The same signal costs 13 +
  N OCI calls per run on the legacy endpoint and none on `/v1`.
- **`AGENTS.md` carries the invariants.** None of the five rules in step 1 is
  derivable from the code, and breaking any of them costs history or money.
