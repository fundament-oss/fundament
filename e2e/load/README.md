# Load tests

k6 scenarios against the local deployment (`just cluster-start`, `just deploy local`).
They model a workshop of about 100 participants, most of whom drive the CLI through
an AI agent. A run fails on a failed request, a wrong answer, a database limit, or
an end state that is not clean.

| Scenario | What it models | Content checks |
|---|---|---|
| `agents` | Per participant, an agent making 2–4 CLI invocations at a time, each with its own API key exchange and a new connection, as functl does; sometimes a project create and delete. Plus an open console per participant | every list and get returns only the participant's own organization; a new project's grant arrives (p95 < 500 ms); a deleted project is gone; another organization refuses |
| `browse` | A workshop start (5 logins/s for a minute), then 150 and 300 open consoles | none; requests only |
| `probe` | A functional check to run alone or during a disruption: logins, reads, a project create, share, delete cycle, a browser login through Dex | the reads return acme-corp's data, a grant reaches Bart within 15 s, Cindy sees no members, Bart loses the project within 15 s of its delete |

## Run

```sh
just e2e load-seed 20 5       # 20 organizations of 5 participants
just e2e load agents          # one agent and one console per participant, 2 min ramp, 10 min hold
just e2e load-seed 60 5       # 300 participants, the margin a 100-participant event should hold
just e2e load agents -e DURATION=5m
just e2e load browse
MAX_OUTAGE=10 just e2e load probe -e DURATION=3m -e RATE=5
just e2e load-clean           # delete the load-NN organizations
```

`just deploy local` resets the databases, so seed again after each deploy. Seeding
again without a deploy reuses the organizations and issues new API keys. `agents`
refuses more agents than seeded participants: two agents on one participant would
share its key and rows, which no real participant does.

`just e2e load-volume` adds 2 million processed rows to the authz outbox, the history
of an installation that has run for a while. Problems with how authz-worker finds its
next row only show on such a table.

## Pass or fail

A run passes when all of these hold:

- **k6 thresholds**:
  - `agents` and `browse`: no failed request, and p95 latency per call under `P95_MS` (500 ms).
  - `agents` and `probe`: no failed content check, and the checks actually ran: a run whose requests all fail cannot pass on zero content failures.
  - `probe`: no dropped iterations.
- **The database**, sampled every 2 s by the recipe:
  - connections stay under 90 % of `max_connections` (`MAX_CONN`);
  - the oldest unprocessed authz outbox row stays under 5 s (`MAX_LAG`). Beyond that, organization-api's circuit breaker refuses every request.
- **The end state**, after authz-worker has caught up: no unprocessed or failed outbox rows, and no project of this run left undeleted.
- **`probe` only**: no call fails for longer than `MAX_OUTAGE` seconds. A disruption may refuse requests for a while, but must not leave anything in a wrong state.

After k6's summary, `report.ts` prints:
- the failures per call and status, with an example;
- for `probe`, the longest outage per call;
- the database peaks, with connections per database user;
- the end state.

Everything lands in `e2e/reports/load/`:
- `<scenario>-events.jsonl`: every failure;
- `<scenario>-db.csv`: the samples;
- `<scenario>-verify.txt`;
- the k6 summary.

## What it does not cover

- **Hardware:** k6 shares the machine with the cluster, and agents wait for each response, so the load falls when the stack slows. A result holds for this machine, not for production hardware.
- **kubectl:** the local kube-api-proxy runs in mock mode. Shoot tokens, watches, `logs -f` and exec need the test-env VM with a real Gardener.
- **Pod and node loss:** run `probe` while you cause it yourself; the dev cluster has one node.
- **Writes:** besides projects and project members, other writes (namespaces, node pools, members of an organization) are not exercised.
- **Other environments:** the seed signs sessions with authn-api's JWT secret and reaches the database directly, so it runs only against a local deployment.

## Settings

Pass these to k6 with `-e NAME=value`; `MAX_OUTAGE`, `MAX_LAG` and `MAX_CONN` go in front of `just`.

| Variable | Scenario | Default |
|---|---|---|
| `AGENTS`, `CONSOLES` | agents | one per participant |
| `DURATION`, `RAMP` | agents, probe | `10m` and `2m`; probe `5m`. Use minutes or seconds, not both |
| `THINK_MIN`, `THINK_MAX` | agents | 0.5 s and 3 s between an agent's steps, shorter than an LLM turn |
| `REUSE=1` | agents | keep connections open instead of one per request |
| `PHASES`, `STRESS_MAX` | browse | `storm,browse`; add `stress` to ramp page views to `STRESS_MAX` (400/s) until the stack gives out |
| `RATE` | probe | read cycles per second, 1 |
| `P95_MS` | all | 500 |
| `BASE_DOMAIN`, `PORT` | all | `fundament.localhost`, `8443` |
