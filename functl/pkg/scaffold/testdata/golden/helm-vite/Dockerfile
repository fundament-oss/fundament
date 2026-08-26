
# The console UI is built first; Vite writes its output to ../console, which the
# Go build then embeds.
FROM oven/bun:1-alpine AS console-ui
WORKDIR /ui
# bun.lock is globbed because a freshly scaffolded project has none yet. Run
# `bun install` in console-ui/, commit the lockfile, then tighten this to
# `bun install --frozen-lockfile` so image builds are reproducible.
COPY console-ui/package.json console-ui/bun.lock* ./
RUN bun install
COPY console-ui/ ./
RUN bun run build

FROM golang:1.26.6-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=console-ui /console ./console
RUN CGO_ENABLED=0 go build -o /demo-plugin .

FROM alpine:3.21
# The plugin installs its chart with Helm at run time.
RUN apk add --no-cache helm
COPY --from=build /demo-plugin /usr/local/bin/demo-plugin
COPY definition.yaml /app/definition.yaml
WORKDIR /app
ENTRYPOINT ["demo-plugin"]
