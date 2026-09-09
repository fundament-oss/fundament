FROM golang:1.26.6-alpine
RUN go install github.com/air-verse/air@v1.65.0
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY marketplace-admin-api/ ./marketplace-admin-api/
COPY marketplace-api/ ./marketplace-api/
COPY common/ ./common/
# Bake $GOCACHE at build time so air's first in-container build is a cache hit
# (compile happens here, throttled by skaffold concurrency — not as a runtime storm).
RUN go build -o /dev/null ./marketplace-admin-api/cmd/fun-marketplace-admin-api
CMD ["air", "-c", "marketplace-admin-api/hotreload/.air.toml"]
