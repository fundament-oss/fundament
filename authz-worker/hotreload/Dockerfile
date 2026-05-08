FROM golang:1.26.3-alpine
RUN go install github.com/air-verse/air@v1.65.0
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY authz-worker/ ./authz-worker/
COPY common/ ./common/
CMD ["air", "-c", "authz-worker/hotreload/.air.toml"]
