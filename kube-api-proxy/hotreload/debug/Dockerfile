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

FROM golang:1.26.5-alpine
RUN go install github.com/air-verse/air@v1.64.5 && \
    go install github.com/go-delve/delve/cmd/dlv@v1.26.1
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY kube-api-proxy/ ./kube-api-proxy/
COPY common/ ./common/
COPY plugins/ ./plugins/
COPY --from=console-ui /console ./plugins/openfsc/console
RUN chmod +x /src/kube-api-proxy/hotreload/debug/start.sh
EXPOSE 2345
CMD ["air", "-c", "kube-api-proxy/hotreload/debug/.air.toml"]
