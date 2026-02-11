_default:
    @just --list e2e

# Install e2e dependencies
install:
    bun install && bunx playwright install chromium

# Run all e2e tests (requires local cluster to be running)
test:
    bun run test

# Run e2e tests with browser visible
test-headed:
    bun run test:headed

# Run e2e tests in debug mode (step through)
test-debug:
    bun run test:debug

# Run specific e2e feature
test-feature feature:
    bun run test -- features/{{ feature }}.feature

# Run e2e tests with specific tags (e.g., @smoke, @login)
test-tags tags:
    bun run test:tags "{{ tags }}"
