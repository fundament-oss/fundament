FROM golang:1.26.1-alpine
RUN go install github.com/air-verse/air@v1.64.5 && \
    go install github.com/go-delve/delve/cmd/dlv@v1.26.1
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY kube-api-proxy/ ./kube-api-proxy/
COPY common/ ./common/
RUN chmod +x /src/kube-api-proxy/hotreload/debug/start.sh
EXPOSE 2345
CMD ["air", "-c", "kube-api-proxy/hotreload/debug/.air.toml"]
