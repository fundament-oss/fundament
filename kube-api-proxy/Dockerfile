FROM golang:1.26.2-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY kube-api-proxy/ ./kube-api-proxy/
COPY common/ ./common/
RUN CGO_ENABLED=0 go build -o /bin/fun-kube-api-proxy ./kube-api-proxy/cmd/fun-kube-api-proxy

FROM scratch
COPY --from=builder /bin/fun-kube-api-proxy /
ENTRYPOINT ["/fun-kube-api-proxy"]
