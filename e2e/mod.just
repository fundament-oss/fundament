_default:
    @just --list e2e

# Install e2e dependencies
install:
    bun install && bunx playwright install chromium

# Run all e2e tests (requires local cluster to be running)
test:
    NODE_EXTRA_CA_CERTS="$(mkcert -CAROOT)/rootCA.pem" bun run test

# Run e2e tests with browser visible
test-headed:
    NODE_EXTRA_CA_CERTS="$(mkcert -CAROOT)/rootCA.pem" bun run test:headed

# Run e2e tests in debug mode (step through)
test-debug:
    NODE_EXTRA_CA_CERTS="$(mkcert -CAROOT)/rootCA.pem" bun run test:debug

# Run specific e2e feature
test-feature feature:
    NODE_EXTRA_CA_CERTS="$(mkcert -CAROOT)/rootCA.pem" bun run test -- features/{{ feature }}.feature

# Run e2e tests with specific tags (e.g., @smoke, @login), from the full suite; @real-gardener ones skip outside the test-env VM
test-tags tags:
    NODE_EXTRA_CA_CERTS="$(mkcert -CAROOT)/rootCA.pem" bun run test:tags "{{ tags }}"

# --- Load tests (load/README.md) ---

load_ctx := "k3d-fundament"
load_base64 := if os() == "macos" { "base64 -D" } else { "base64 -d" }

# Seed TEAMS organizations of PER participants for `just e2e load agents`; seeding again reuses them and issues new API keys
load-seed teams="20" per="5": _load-funops
    #!/usr/bin/env bash
    set -euo pipefail
    export FUNOPS="$PWD/reports/load/funops" FUNOPS_DB_HOST=localhost:54328
    export FUNOPS_DB_PASSWORD=$(kubectl --context {{ load_ctx }} get secret -n fundament db-fun-operator -o jsonpath='{.data.password}' | {{ load_base64 }})
    export JWT_SECRET=$(kubectl --context {{ load_ctx }} get deploy -n fundament authn-api -o jsonpath='{.spec.template.spec.containers[0].env[?(@.name=="JWT_SECRET")].value}')
    NODE_EXTRA_CA_CERTS="$(mkcert -CAROOT)/rootCA.pem" bun load/seed.ts {{ teams }} {{ per }} reports/load/participants.json

# Add ROWS processed rows to the authz outbox: the history of an installation that has run for a while
load-volume rows="2000000":
    @case "{{ rows }}" in "" | *[!0-9]*) echo "rows must be a whole number" >&2; exit 1 ;; esac
    @just e2e _load-sql "insert into authz.outbox (project_id, created, processed, status) select (select id from tenant.projects where deleted is null order by created limit 1), now() - interval '30 days' + g * interval '1 second', now() - interval '30 days' + g * interval '1 second', 'completed' from generate_series(1, {{ rows }}) g"

# Run a load scenario (agents, browse, probe); extra args go to k6, e.g. -e AGENTS=300. Samples the database meanwhile, then checks the end state
load scenario *args:
    #!/usr/bin/env bash
    set -uo pipefail
    out="$PWD/reports/load"
    mkdir -p "$out"
    rm -f "$out/{{ scenario }}-"{db.csv,events.jsonl,verify.txt}
    run=$(printf '%x' "$(date +%s)")
    start=$(date +%s)
    sql() { just e2e _load-sql "$1"; }
    unprocessed="select count(*) from authz.outbox where processed is null and status in ('pending', 'retrying')"

    # Every 2 s: client connections to the instance (OpenFGA's database too),
    # max_connections, unprocessed outbox rows, the oldest one's age in seconds
    # (what organization-api's breaker reads), failed rows, and connections per
    # database user. A sample is lost when the query fails; report.ts fails on
    # the gap.
    while :; do
        row=$(sql "select (select count(*) from pg_stat_activity where backend_type = 'client backend'), current_setting('max_connections'), ($unprocessed), (select coalesce(extract(epoch from now() - min(created)), 0)::numeric(10, 1) from authz.outbox where processed is null and status in ('pending', 'retrying')), (select count(*) from authz.outbox where status = 'failed'), (select string_agg(u || '=' || n, ' ' order by n desc) from (select usename u, count(*) n from pg_stat_activity where backend_type = 'client backend' group by usename) x)" 2>/dev/null) \
            && echo "$(date +%s),$row" >> "$out/{{ scenario }}-db.csv"
        sleep 2
    done &
    sampler=$!
    trap 'kill $sampler 2>/dev/null' EXIT

    edge=$(docker inspect -f '{{{{(index .NetworkSettings.Networks "k3d-fundament").IPAddress}}' k3d-fundament-server-0)
    docker run --rm --network k3d-fundament -u "$(id -u)" \
        -e EDGE="$edge" -e INSECURE=1 -e RUN="$run" -e MAX_OUTAGE="${MAX_OUTAGE:-}" -e PEOPLE=/reports/participants.json \
        -v "$PWD/load:/scripts:ro" -v "$out:/reports" grafana/k6:1.4.2 \
        run --log-format raw --log-output "file=/reports/{{ scenario }}-events.jsonl" \
        --summary-export "/reports/{{ scenario }}.summary.json" {{ args }} "/scripts/{{ scenario }}.js"
    k6=$?
    kill $sampler 2>/dev/null

    # Let authz-worker finish what the run queued, then record the end state.
    for _ in $(seq 30); do
        left=$(sql "$unprocessed")
        [ "$left" = 0 ] && break
        sleep 2
    done
    {
        echo "outbox_unprocessed=$left"
        echo "outbox_failed=$(sql "select count(*) from authz.outbox where status = 'failed' and created >= to_timestamp($start)")"
        echo "projects_left=$(sql "select count(*) from tenant.projects where deleted is null and (name like 'l$run-%' or name like 'p$run-%')")"
    } > "$out/{{ scenario }}-verify.txt"

    bun load/report.ts "$out" {{ scenario }}
    report=$?
    [ "$k6" = 0 ] && [ "$report" = 0 ] || { echo "load {{ scenario }} failed (k6 exit $k6, report exit $report)" >&2; exit 1; }

_load-funops:
    @mkdir -p reports/load && go build -C .. -o "$PWD/reports/load/funops" ./funops/cmd/funops

# SQL as postgres on the primary of the local database, comma-separated
_load-sql query:
    @kubectl --context {{ load_ctx }} -n fundament exec "$(kubectl --context {{ load_ctx }} -n fundament get pod -l cnpg.io/cluster=db,cnpg.io/instanceRole=primary -o name)" -c postgres -- psql -U postgres -d fundament -AtF, -c "{{ query }}"
