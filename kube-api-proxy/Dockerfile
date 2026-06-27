# Build the OpenFSC console UI (Vite, vanilla TS) → /console. mock mode serves
# these assets from disk for the plugin iframe. The build context excludes
# plugins/openfsc/console (.dockerignore), so this stage is the image's only
# source of them — mirrors plugins/openfsc/Dockerfile.
FROM oven/bun:1-alpine AS console-ui
WORKDIR /ui
COPY plugins/openfsc/console-ui/package.json plugins/openfsc/console-ui/bun.lock ./
RUN bun install --frozen-lockfile
COPY plugins/openfsc/console-ui/ ./
RUN bun run build

FROM golang:1.26.4-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY kube-api-proxy/ ./kube-api-proxy/
COPY common/ ./common/
RUN CGO_ENABLED=0 go build -o /bin/fun-kube-api-proxy ./kube-api-proxy/cmd/fun-kube-api-proxy

COPY plugins/ ./plugins/
COPY --from=console-ui /console ./plugins/openfsc/console
RUN mkdir -p /assets/plugins && \
    for dir in ./plugins/*/console; do \
      [ -d "$dir" ] || continue; \
      name=$(basename "$(dirname "$dir")"); \
      mkdir -p "/assets/plugins/$name"; \
      cp -r "$dir" "/assets/plugins/$name/console"; \
    done

FROM scratch
COPY --from=builder /bin/fun-kube-api-proxy /
COPY --from=builder /assets/plugins /plugins
ENTRYPOINT ["/fun-kube-api-proxy"]
