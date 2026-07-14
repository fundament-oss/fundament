FROM golang:1.26.4-alpine
RUN go install github.com/air-verse/air@v1.65.0
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY dcim-authn-api/ ./dcim-authn-api/
COPY common/ ./common/
# Bake $GOCACHE at build time so air's first in-container build is a cache hit
# (compile happens here, throttled by skaffold concurrency — not as a runtime storm).
RUN go build -o /dev/null ./dcim-authn-api/cmd/dcim-authn-api
CMD ["air", "-c", "dcim-authn-api/hotreload/.air.toml"]
