FROM golang:1.26.5-alpine
RUN go install github.com/air-verse/air@v1.65.0
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY authn-api/ ./authn-api/
COPY common/ ./common/
COPY plugin-proxy/ ./plugin-proxy/
# Bake $GOCACHE at build time so air's first in-container build is a cache hit
# (compile happens here, throttled by skaffold concurrency — not as a runtime storm).
RUN go build -o /dev/null ./authn-api/cmd/fun-authn-api
CMD ["air", "-c", "authn-api/hotreload/.air.toml"]
