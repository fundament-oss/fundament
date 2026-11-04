FROM golang:1.27.3-alpine
RUN go install github.com/air-verse/air@v1.65.0
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY authz-api/ ./authz-api/
COPY common/ ./common/
# Bake $GOCACHE at build time so air's first in-container build is a cache hit.
RUN go build -o /dev/null ./authz-api/cmd/fun-authz-api
CMD ["air", "-c", "authz-api/hotreload/.air.toml"]
