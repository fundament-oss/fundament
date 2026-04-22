FROM golang:1.26-alpine AS builder
RUN apk add --no-cache git
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /bin/gateway-api ./plugins/gateway-api

FROM alpine:3.21
RUN apk add --no-cache curl && \
    curl -fsSL https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | sh && \
    apk del curl
COPY --from=builder /bin/gateway-api /gateway-api
COPY plugins/gateway-api/definition.yaml /app/definition.yaml
WORKDIR /app
ENTRYPOINT ["/gateway-api"]
