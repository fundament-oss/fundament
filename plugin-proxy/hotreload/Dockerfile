# Bun sdk-builder — bundles the plugin-sdk + NLDD Design System JS/CSS so
# plugin-proxy can serve them at /plugins/sdk/v1/ during hotreload dev sessions
# (matches the production Dockerfile's setup).
FROM oven/bun:1.3-alpine AS sdk-builder
WORKDIR /console-frontend
COPY console-frontend/package.json console-frontend/bun.lock* ./
RUN bun install --frozen-lockfile
COPY console-frontend/src/plugin-sdk/ ./src/plugin-sdk/
RUN mkdir -p public/plugin-ui && bun run build:plugin-ui

FROM golang:1.26.5-alpine
RUN go install github.com/air-verse/air@v1.65.0
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY plugin-proxy/ ./plugin-proxy/
COPY plugin-controller/ ./plugin-controller/
COPY common/ ./common/
# Bake $GOCACHE at build time so air's first in-container build is a cache hit
# (compile happens here, throttled by skaffold concurrency — not as a runtime storm).
RUN go build -o /dev/null ./plugin-proxy/cmd/fun-plugin-proxy
COPY --from=sdk-builder /console-frontend/public/plugin-ui/ /sdk/v1/
ENV PLUGIN_SDK_DIR=/sdk/v1
CMD ["air", "-c", "plugin-proxy/hotreload/.air.toml"]
