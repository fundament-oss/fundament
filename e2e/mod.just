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

# Seed TEAMS organizations of PER participants for `just e2e load agents`; seeding again reuses them and issues new API keys
load-seed teams="20" per="5": _load-funops
    go run ./load seed {{ teams }} {{ per }} --funops reports/load/funops

# Add ROWS processed rows to the authz outbox: the history of an installation that has run for a while
load-volume rows="2000000":
    go run ./load volume {{ rows }}

# Run a load scenario (agents, browse, probe); extra args go to k6, e.g. -e AGENTS=300. Samples the database meanwhile, then checks the end state
load scenario *args:
    go run ./load run {{ scenario }} -- {{ args }}

# Judge a finished run again from its files, e.g. with other limits in front of just
load-report scenario:
    go run ./load report {{ scenario }}

_load-funops:
    @mkdir -p reports/load && go build -C .. -o "$PWD/reports/load/funops" ./funops/cmd/funops
