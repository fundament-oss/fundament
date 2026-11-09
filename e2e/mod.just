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

# Run e2e tests with specific tags (e.g., @smoke, @login)
test-tags tags:
    NODE_EXTRA_CA_CERTS="$(mkcert -CAROOT)/rootCA.pem" bun run test:tags "{{ tags }}"

# Run the @real-gardener node pool provisioning test (needs a real Gardener;
# see e2e/features/node-pool-provisioning.feature for setup). The default
# profile excludes this tag, so it uses its own cucumber.json profile rather
# than `test-tags @real-gardener` (which would AND against that exclusion
# and always match zero scenarios).
test-real-gardener:
    NODE_EXTRA_CA_CERTS="$(mkcert -CAROOT)/rootCA.pem" bun run test:real-gardener
