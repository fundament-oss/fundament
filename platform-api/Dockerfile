# One Go tree, two deployables (FUN-23): BINARY picks the command,
# platform-public-api or platform-admin-api.
FROM golang:1.27.3-alpine AS builder
ARG BINARY
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY platform-api/ ./platform-api/
COPY common/ ./common/
RUN test -n "$BINARY" || (echo "BINARY build argument is required" >&2 && exit 1)
RUN CGO_ENABLED=0 go build -o /build/bin/platform-api ./platform-api/cmd/fun-${BINARY}

FROM scratch
COPY --from=builder /build/bin/platform-api /
ENTRYPOINT ["/platform-api"]
