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

FROM golang:1.26.4-alpine
RUN go install github.com/air-verse/air@v1.65.0
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY kube-api-proxy/ ./kube-api-proxy/
COPY common/ ./common/
COPY plugins/ ./plugins/
COPY --from=console-ui /console ./plugins/openfsc/console
CMD ["air", "-c", "kube-api-proxy/hotreload/.air.toml"]
