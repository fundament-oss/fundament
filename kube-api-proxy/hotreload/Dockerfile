FROM golang:1.26.2-alpine
RUN go install github.com/air-verse/air@latest
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY kube-api-proxy/ ./kube-api-proxy/
COPY common/ ./common/
CMD ["air", "-c", "kube-api-proxy/hotreload/.air.toml"]
