# Build the console UI (Vite, vanilla TS) → plugins/gateway-api/envoy-gateway/console,
# which console.go embeds via go:embed. The NLDD Design System is NOT bundled here:
# the app loads it at runtime from the Console's shared /plugin-ui bundle.
FROM oven/bun:1-alpine AS console-ui
WORKDIR /ui
COPY plugins/gateway-api/envoy-gateway/console-ui/package.json plugins/gateway-api/envoy-gateway/console-ui/bun.lock ./
RUN bun install --frozen-lockfile
COPY plugins/gateway-api/envoy-gateway/console-ui/ ./
RUN bun run build

FROM golang:1.26.5 AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
# Overlay the freshly built console assets. The build context excludes the built
# console dir (.dockerignore), so this stage is the only source of the embedded UI.
COPY --from=console-ui /console ./plugins/gateway-api/envoy-gateway/console
RUN CGO_ENABLED=0 go build -o /bin/gateway-api-envoy ./plugins/gateway-api/envoy-gateway

FROM alpine:3.22

RUN apk add --no-cache helm

COPY --from=build /bin/gateway-api-envoy /usr/local/bin/gateway-api-envoy
COPY plugins/gateway-api/envoy-gateway/definition.yaml /app/definition.yaml

WORKDIR /app
ENTRYPOINT ["gateway-api-envoy"]
