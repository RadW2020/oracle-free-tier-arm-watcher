# Investigate why the checks timed out at 17:14

**Kind:** read-only investigation · **Use case:** UC3 in `docs/agent-use-cases.md`
**Run:** Claude Code (Sonnet), headless, connected only to the watcher's MCP
server. It had no shell, no file access and no web. Scenario:
`incident-2026-09-17`, the synthetic reconstruction of the real incident, served
by `docker compose up`. Recorded on 2026-09-25: 5 turns, $0.14.

## The request

> Our Checkly checks started timing out around 17:14 on 17 September (Madrid
> time), and everything on the VM felt slow for a while after that. Was the host
> the cause? What happened?

Before this interface existed, answering this took an evening. The operator used
the `oci` CLI with the tenancy signing key, hand-written MQL queries and a
UTC/CEST conversion. It also meant knowing that Monitoring labels buckets with
the *end* of their interval, and that `oci_computeagent` counts Docker's
interfaces (see `POSTMORTEM-2026-09-17.md` §6).

## What the agent did

The agent converted the time first ("17:14 Madrid (CEST, UTC+2) is 15:14 UTC"),
then made four calls:

| # | Call | What came back |
|---|---|---|
| 1 | `get_saturation_timeline {start: 14:30Z, end: 18:30Z, resolution: 1m}` | **Error**, `invalid_argument` on field `includePoints`: "this request would return ~1200 points (max 720)". The hint: "ask for fewer metrics, a coarser resolution or a shorter window, or set includePoints=false to get only the summary". |
| 2 | Same window, `includePoints: false` | Summary only. Two throttle-drop intervals: **15:14–15:20Z (81,000 packets)** and 17:27–17:32Z (45,000). `dataLag` says the last 3 minutes aren't published yet. |
| 3 | `start: 14:55Z, end: 15:45Z, resolution: 1m` | Full points around the first burst: ingress ~8 → ~397 MB/min, 13,500 drops/min for 6 minutes, CPU up to 35.5 % from 15:21. |
| 4 | `start: 15:00Z, end: 18:30Z, resolution: 5m` | The recovery: CPU stays high until ~18:10Z and a second, larger surge arrives at 17:25Z. |

## What it answered (trimmed)

> **Yes — the host was the cause.**
>
> **17:14–17:20 Madrid (15:14–15:20 UTC): inbound traffic spike.** Ingress
> jumped from a normal baseline of ~8 MB/min to ~397 MB/min, sustained for 6
> minutes. This exceeded the instance's shape bandwidth, so OCI's VNIC ingress
> shaper started dropping inbound packets — 13,500/min, 81,000 total. Throttle
> drops affect *every* service on the host, including TCP SYNs, which is exactly
> what shows up as Checkly connection timeouts.
>
> **17:21 Madrid onward: CPU pinned at 35.5 %** … until 20:10 Madrid.
>
> **19:25–19:35 Madrid: second, larger surge** … a second round of throttle
> drops (45,000 packets).
>
> **Memory:** flat at ~30–31 % the entire time — not a factor.
>
> I don't have visibility into what generated the traffic — that'd need VCN flow
> logs or the app's own access logs.

These are the conclusions the human postmortem reached, and the agent reached
them without the signing key. It also marked the edge of what it could see
instead of guessing that the traffic came from AIDRA.

## What this shows about the interface

- **The error taught the agent how to succeed.** Call 1 was too big for a
  model's context. The error named the field, gave the limit and listed three
  ways out, and the agent took one on the next call. Nobody scripted that
  recovery.
- **The summary carried the finding.** `throttleDropIntervals` gave the agent
  the answer before it looked at a single point; it fetched points only to
  confirm.
- **Domain knowledge lives in the interface, not in the prompt.**
  - "Drops mean every service loses SYNs" and "times are UTC" come from the MCP
    server instructions and the tool descriptions.
  - "Buckets are labelled with their end" is spelled out in `bucketLabel`.
  - The lag is spelled out in `dataLag`.
- **Reads only.** The audit log shows four `get_saturation_timeline` calls under
  client `demo-agent` and nothing else.

The same task, with a JSON answer contract, is eval case `incident-17-09` (see
`evals/cases/05-incident-17-09.yaml`). Its graders check that the queried window
covers 15:14–15:20Z, which catches an agent that forgets the timezone.
