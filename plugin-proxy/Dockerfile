FROM golang:1.26.5-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY plugin-proxy/ ./plugin-proxy/
COPY plugin-controller/ ./plugin-controller/
COPY common/ ./common/
RUN CGO_ENABLED=0 go build ./plugin-proxy/cmd/fun-plugin-proxy

FROM scratch
COPY --from=builder /build/fun-plugin-proxy /
ENTRYPOINT ["/fun-plugin-proxy"]
