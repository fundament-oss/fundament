FROM golang:1.25.5-alpine
RUN go install github.com/air-verse/air@latest
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cluster-worker/ ./cluster-worker/
COPY common/ ./common/
CMD ["air", "-c", "cluster-worker/hotreload/.air.toml"]
