FROM golang:1.26.4-alpine
RUN go install github.com/air-verse/air@v1.65.0
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY plugin-proxy/ ./plugin-proxy/
COPY plugin-controller/ ./plugin-controller/
COPY common/ ./common/
CMD ["air", "-c", "plugin-proxy/hotreload/.air.toml"]
