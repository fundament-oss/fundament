FROM golang:1.26.1-alpine
RUN go install github.com/air-verse/air@latest
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY kube-proxy/ ./kube-proxy/
COPY common/ ./common/
CMD ["air", "-c", "kube-proxy/hotreload/.air.toml"]
