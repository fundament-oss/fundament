FROM golang:1.26.5-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY plugin-proxy/ ./plugin-proxy/
COPY plugin-controller/ ./plugin-controller/
COPY common/ ./common/
RUN CGO_ENABLED=0 go build ./plugin-proxy/cmd/fun-plugin-proxy

# Build the plugin SDK + NLDD Design System bundles here so the final image can
# serve them at /plugins/sdk/v1/ (FUN-17 requires the SDK come from the
# plugin-proxy origin under script-src 'self'; the NLDD bundle rides the same
# origin so plugin console-ui views can load their <nldd-*> components). Runs
# console-frontend's `build:plugin-ui` so the bundles stay identical to the ones
# the Console serves at /plugin-ui/.
FROM oven/bun:1.3-alpine AS sdk-builder
WORKDIR /console-frontend
COPY console-frontend/package.json console-frontend/bun.lock* ./
RUN bun install --frozen-lockfile
COPY console-frontend/src/plugin-sdk/ ./src/plugin-sdk/
RUN mkdir -p public/plugin-ui && bun run build:plugin-ui

FROM scratch
COPY --from=builder /build/fun-plugin-proxy /
COPY --from=sdk-builder /console-frontend/public/plugin-ui/ /sdk/v1/
ENV PLUGIN_SDK_DIR=/sdk/v1
ENTRYPOINT ["/fun-plugin-proxy"]
