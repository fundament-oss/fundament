FROM golang:1.26.5-alpine
RUN go install github.com/air-verse/air@v1.65.0
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY organization-api/ ./organization-api/
COPY common/ ./common/
COPY plugin-sdk/ ./plugin-sdk/
# Bake $GOCACHE at build time so air's first in-container build is a cache hit
# (compile happens here, throttled by skaffold concurrency — not as a runtime storm).
RUN go build -o /dev/null ./organization-api/cmd/fun-organization-api
CMD ["air", "-c", "organization-api/hotreload/.air.toml"]
