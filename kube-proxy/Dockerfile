FROM golang:1.26.1-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY kube-proxy/ ./kube-proxy/
COPY common/ ./common/
RUN CGO_ENABLED=0 go build -o /bin/fun-kube-proxy ./kube-proxy/cmd/fun-kube-proxy

FROM scratch
COPY --from=builder /bin/fun-kube-proxy /
ENTRYPOINT ["/fun-kube-proxy"]
