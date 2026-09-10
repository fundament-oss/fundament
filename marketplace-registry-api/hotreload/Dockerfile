FROM golang:1.26.6-alpine
RUN go install github.com/air-verse/air@v1.65.0
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY marketplace-registry-api/ ./marketplace-registry-api/
COPY marketplace-api/ ./marketplace-api/
COPY plugin-sdk/pluginruntime/ ./plugin-sdk/pluginruntime/
COPY common/ ./common/
# Bake $GOCACHE at build time so air's first in-container build is a cache hit
# (compile happens here, throttled by skaffold concurrency — not as a runtime storm).
RUN go build -o /dev/null ./marketplace-registry-api/cmd/fun-marketplace-registry-api
CMD ["air", "-c", "marketplace-registry-api/hotreload/.air.toml"]
