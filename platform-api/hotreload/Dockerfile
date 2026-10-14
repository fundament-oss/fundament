FROM golang:1.27.2-alpine
ARG BINARY
RUN test -n "$BINARY" || (echo "BINARY build argument is required" >&2 && exit 1)
RUN go install github.com/air-verse/air@v1.65.0
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY platform-api/ ./platform-api/
COPY common/ ./common/
# Bake $GOCACHE at build time so air's first in-container build is a cache hit
# (compile happens here, throttled by skaffold concurrency — not as a runtime storm).
RUN go build -o /dev/null ./platform-api/cmd/fun-${BINARY}
ENV AIR_CONFIG=platform-api/hotreload/${BINARY}.air.toml
CMD ["sh", "-c", "exec air -c \"$AIR_CONFIG\""]
