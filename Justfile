_default:
    @just --list

# Watch for changes to .d2 files and re-generate .svgs
watch-d2:
    d2 --theme=0 --dark-theme=200 --watch docs/assets/*.d2

# Format all code and text in this repo
fmt:
    @find . -type f \( -name "*.md" -o -name "*.d2" \) -exec sed -i 's/𝑒𝑛𝑡𝑒𝑟𝑝𝑟𝑖𝑠𝑒/𝑒𝑛𝑡𝑒𝑟𝑝𝑟𝑖𝑠𝑒/g' {} +
    d2 fmt docs/assets/*.d2
    # TODO md fmt
    # TODO go fmt

# Run Dex OIDC provider locally
dex:
    docker run --rm -p 5556:5556 -v {{justfile_directory()}}/dex-config.yaml:/etc/dex/config.yaml:ro ghcr.io/dexidp/dex:v2.44.0 dex serve /etc/dex/config.yaml

# Run authn-api locally
authn-api:
    cd authn-api && JWT_SECRET=abc LISTEN_ADDR=:10100 go run .

# Run organization-api locally
organization-api:
    cd organization-api && JWT_SECRET=abc LISTEN_ADDR=:10101 go run .

fe:
    cd login-frontend && pnpm run dev

generate:
    just sqlc && just proto

# Generate protobuf code for authn-api
proto:
    cd authn-api/proto && buf generate
    cd organization-api/proto && buf generate

# Generate all sqlc code
sqlc:
    cd authn-api/sqlc && sqlc generate
    cd organization-api/sqlc && sqlc generate

# Lint all Go code
lint:
    cd authn-api && golangci-lint run ./...
    cd organization-api && golangci-lint run ./...

db:  
    docker run --rm -p 5432:5432 \
    -e POSTGRES_DB=fundament \
    -e POSTGRES_PASSWORD=postgres \
    -v $(pwd)/db/fundament.sql:/docker-entrypoint-initdb.d/01-schema.sql \
    -v $(pwd)/init-users.sql:/docker-entrypoint-initdb.d/02-users.sql \
    postgres:18