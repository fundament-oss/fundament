FROM golang:1.26.3-alpine
RUN go install github.com/air-verse/air@v1.65.0
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY kube-api-proxy/ ./kube-api-proxy/
COPY common/ ./common/
COPY plugins/ ./plugins/
CMD ["air", "-c", "kube-api-proxy/hotreload/.air.toml"]
