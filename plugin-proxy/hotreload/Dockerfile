# Bun sdk-builder — bundles the plugin-sdk JS/CSS so plugin-proxy can serve
# it at /plugins/sdk/v1/ during hotreload dev sessions (matches the
# production Dockerfile's setup).
FROM oven/bun:1.3-alpine AS sdk-builder
WORKDIR /console-frontend
COPY console-frontend/package.json console-frontend/bun.lock* ./
RUN bun install --frozen-lockfile
COPY console-frontend/src/plugin-sdk/ ./src/plugin-sdk/
RUN mkdir -p public/plugin-ui && \
    bun build src/plugin-sdk/plugin-sdk.ts \
      --outfile public/plugin-ui/plugin-sdk.js \
      --target browser --format iife --minify && \
    bun src/plugin-sdk/build-css.ts

FROM golang:1.26.4-alpine
RUN go install github.com/air-verse/air@v1.65.0
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY plugin-proxy/ ./plugin-proxy/
COPY plugin-controller/ ./plugin-controller/
COPY common/ ./common/
COPY --from=sdk-builder /console-frontend/public/plugin-ui/ /sdk/v1/
ENV PLUGIN_SDK_DIR=/sdk/v1
CMD ["air", "-c", "plugin-proxy/hotreload/.air.toml"]
