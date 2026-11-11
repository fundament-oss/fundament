
# The console UI is built first; Vite writes its output to ../console, which the
# Go build then embeds. The output is the same for every platform, so this stage
# runs natively on the build machine.
FROM --platform=$BUILDPLATFORM oven/bun:1-alpine AS console-ui
WORKDIR /ui
# bun.lock is globbed because a freshly scaffolded project has none yet. Run
# `bun install` in console-ui/, commit the lockfile, then tighten this to
# `bun install --frozen-lockfile` so image builds are reproducible.
COPY console-ui/package.json console-ui/bun.lock* ./
RUN bun install
COPY console-ui/ ./
RUN bun run build

# Go cross-compiles for the target platform on the build machine's own
# architecture: running the compiler under emulation is slow and can crash.
FROM --platform=$BUILDPLATFORM golang:1.27.4-alpine AS build
ARG TARGETOS TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=console-ui /console ./console
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o /demo-plugin .

FROM alpine:3.21
COPY --from=build /demo-plugin /usr/local/bin/demo-plugin
COPY definition.yaml /app/definition.yaml
WORKDIR /app
ENTRYPOINT ["demo-plugin"]
