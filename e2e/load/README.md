# Load tests

Load scenarios against the local deployment (`just cluster-start`, `just deploy local`), to
find where fundament breaks under the workloads we expect: a workshop of about 100
participants, most of whom drive the CLI through an AI agent. They are not a production
capacity test. A run fails on a failed request, a wrong answer, a database limit, or an end
state that is not clean; every limit is a setting, so a run can look for the point where one
gives out.

k6 generates the traffic (`*.js`). `go run ./load` (`*.go`) does the rest: it seeds the
participants, samples the database during the run, records the end state and judges the run.

| Scenario | What it models | Content checks |
|---|---|---|
| `agents` | One VU per participant: an agent making 2–4 CLI invocations at a time, each with its own API key exchange and a new connection, as functl does; sometimes a project create and delete. Between its steps the VU polls the participant's open console | every list and get returns only the participant's own organization; a new project's grant arrives; a deleted project is gone; another organization, and another organization's cluster, refuse |
| `browse` | A workshop start (5 logins/s for a minute), then 150 and 300 open consoles | none; requests only |
| `probe` | A functional check to run alone or during a disruption: logins, reads, a project create, share, delete cycle, a browser login through Dex | the reads return acme-corp's data, a grant reaches Bart within `GRANT_WAIT_S`, Cindy sees no members, Bart loses the project within `GRANT_WAIT_S` of its delete |

## Run

```sh
just e2e load-seed 20 5       # 20 organizations of 5 participants
just e2e load agents          # one agent and one console per participant, 2 min ramp, 10 min hold
just e2e load-seed 60 5       # 300 participants, the margin a 100-participant event should hold
just e2e load agents -e DURATION=5m
just e2e load browse
MAX_OUTAGE=10 just e2e load probe -e DURATION=3m -e RATE=5
MAX_CONN=0.5 just e2e load-report agents   # judge the last agents run again
```

Ctrl-C stops k6; the end state and the report still follow. A second Ctrl-C stops at once.

`just deploy local` resets the databases and with them the participants, so seed again after
each deploy. Seeding again without a deploy reuses the organizations, keeps the first `PER`
participants of each and issues new API keys. `agents` refuses more VUs than seeded
participants: two VUs on one participant would share its key and rows, which no real
participant does.

`just e2e load-volume` adds 2 million processed rows to the authz outbox, the history of an
installation that has run for a while. Problems with how authz-worker finds its next row
only show on such a table.

The commands run only against a k3d cluster (`--context`, default `k3d-fundament`): the seed
signs sessions with authn-api's JWT secret and writes to the database directly.

## Pass or fail

A run passes when all of these hold:

- **k6 thresholds**:
  - every content check holds, and checks actually ran: a run whose requests all fail cannot pass on zero content failures. In `probe`, the read cycles must run at least `CHECK_SHARE` of their checks;
  - at most `FAIL_RATE` of the requests fail, in `agents`, in each `browse` phase but stress, and in `probe` without `MAX_OUTAGE`;
  - `agents`: a new project's grant arrives within `GRANT_P95_MS` and `GRANT_P99_MS`;
  - `probe`: at most `MAX_DROPPED` iterations are dropped;
  - with `P95_MS` set, p95 latency per call stays under it. Without it, k6 prints each call's latency and judges none: latency depends on the machine k6 shares with the cluster.
- **The database**, sampled every 2 s over one psql session, with no gap over `MAX_GAP`:
  - client connections to the instance stay under `MAX_CONN` of `max_connections`;
  - the oldest unprocessed authz outbox row stays under `MAX_LAG`. Beyond organization-api's `CIRCUIT_BREAKER_THRESHOLD`, its default, the breaker refuses every request.
- **The end state**, after authz-worker has caught up: no unprocessed or failed outbox rows, and no project of this run left undeleted.
- **`probe` with `MAX_OUTAGE`**: no call fails for longer than `MAX_OUTAGE` seconds. A disruption may refuse requests for a while, but must not leave anything in a wrong state.

After k6's summary the report prints:
- the failures per call and status, with an example;
- with `MAX_OUTAGE`, the longest outage per call;
- the database peaks, with connections per database user;
- the end state.

Everything lands in `e2e/reports/load/`:
- `<scenario>-events.jsonl`: every failure;
- `<scenario>-db.csv`: the samples;
- `<scenario>-verify.txt`: the end state;
- the k6 summary.

`participants.json` there holds the participants' API keys and sessions, valid for 24 hours.

## What it does not cover

- **Periodic runs:** nothing runs these scenarios in CI or on a schedule; there is no environment to run them in yet, as for the E2E tests against a real Gardener. A change that breaks a scenario shows at its next run on a developer's machine. The Go side builds and its unit tests run in CI; the k6 scripts call the APIs by path, so a renamed field shows only in a run.
- **Hardware:** k6 shares the machine with the cluster, and agents wait for each response, so the load falls when the stack slows. A result holds for this machine, not for production hardware.
- **The k3d load balancer:** k6 sends its requests to the k3d node's port 443, past the serverlb in front of 127.0.0.1:8443, whose nginx has 1024 file descriptors and drops connections from a few hundred clients.
- **kubectl:** the local kube-api-proxy runs in mock mode. Shoot tokens, watches, `logs -f` and exec need the test-env VM with a real Gardener.
- **Pod and node loss:** run `probe` while you cause it yourself; the dev cluster has one node.
- **Writes:** besides projects and project members, other writes (namespaces, node pools, members of an organization) are not exercised.

## Settings

Pass the k6 settings with `-e NAME=value`. `MAX_OUTAGE` and the database limits go in front of
`just`; `MAX_OUTAGE` reaches k6 too.

| Variable | Scenario | Default |
|---|---|---|
| `AGENTS`, `CONSOLES` | agents | one per participant |
| `DURATION` | agents, probe | agents `10m`, probe `5m`; for probe one unit, such as `90s`, `5m` or `1h` |
| `RAMP` | agents | `2m` |
| `THINK_MIN`, `THINK_MAX` | agents | 0.5 s and 3 s between an agent's steps, shorter than an LLM turn |
| `REUSE=1` | agents | keep connections open instead of one per request |
| `PHASES`, `STRESS_MAX` | browse | `storm,browse`; add `stress` to ramp page views to `STRESS_MAX` (400/s) until the stack gives out |
| `RATE` | probe | read cycles per second, 1 |
| `BASE_DOMAIN`, `PORT` | all | `fundament.localhost`, `8443` |
| **k6 thresholds** | | |
| `P95_MS` | all | unset: no latency limit |
| `FAIL_RATE` | all | 0 |
| `GRANT_P95_MS`, `GRANT_P99_MS` | agents | 500, 2000 |
| `GRANT_WAIT_S` | probe | 15 |
| `CHECK_SHARE` | probe | 0.5 |
| `MAX_DROPPED` | probe | 0 |
| **Report limits** | | |
| `MAX_OUTAGE` | probe | unset: refused requests are left to `FAIL_RATE` |
| `MAX_LAG` | all | organization-api's `CIRCUIT_BREAKER_THRESHOLD` (5 s) |
| `MAX_CONN` | all | 0.9 |
| `MAX_GAP` | all | 10 s |
