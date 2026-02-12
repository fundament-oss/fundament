FROM golang:1.25.5-alpine
RUN go install github.com/air-verse/air@latest
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY authz-worker/ ./authz-worker/
COPY common/ ./common/
CMD ["air", "-c", "authz-worker/hotreload/.air.toml"]
