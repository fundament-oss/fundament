# Build the console UI (Vite, vanilla TS) → plugins/openfsc/console, which
# console.go embeds via go:embed. The NLDD Design System is NOT bundled here: the app
# loads it at runtime from the Console's shared /plugin-ui bundle. See
# docs/funs/FUN-18.adoc.
FROM oven/bun:1-alpine AS console-ui
WORKDIR /ui
COPY plugins/openfsc/console-ui/package.json plugins/openfsc/console-ui/bun.lock ./
RUN bun install --frozen-lockfile
COPY plugins/openfsc/console-ui/ ./
# vite build writes to ../console → /console (outDir is outside the Vite root).
RUN bun run build

FROM golang:1.26.5 AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
# Overlay the freshly built console assets. The build context excludes
# plugins/openfsc/console (.dockerignore), so this is the only source of the
# embedded UI — keeping the image in sync with console-ui/.
COPY --from=console-ui /console ./plugins/openfsc/console
RUN CGO_ENABLED=0 go build -o /openfsc-plugin ./plugins/openfsc/

FROM alpine:3.21

# helm: the plugin installs the prerequisite charts (cert-manager,
# CloudNativePG) and the vendored openfsc-operator chart at runtime.
RUN apk add --no-cache helm

COPY --from=build /openfsc-plugin /usr/local/bin/openfsc-plugin
COPY plugins/openfsc/definition.yaml /app/definition.yaml
# The openfsc-operator chart (FSCInstallation CRD + Deployment + scoped RBAC),
# installed offline by the plugin; the operator image itself comes from
# FUNP_OPERATOR_IMAGE.
COPY openfsc-operator/chart /operator-chart

WORKDIR /app
ENTRYPOINT ["openfsc-plugin"]
