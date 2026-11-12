FROM golang:1.27.4-alpine AS builder
RUN apk add --no-cache ca-certificates
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY marketplace-registry-api/ ./marketplace-registry-api/
# The API contract lives with neither service: catalog.v1 and registry.v1 both
# import marketplace.v1.
COPY marketplace-api/ ./marketplace-api/
# CreatePluginVersion parses and hashes the pushed manifest with pluginruntime.
COPY plugin-sdk/pluginruntime/ ./plugin-sdk/pluginruntime/
COPY common/ ./common/
RUN CGO_ENABLED=0 go build -o /build/bin/marketplace-registry-api ./marketplace-registry-api/cmd/fun-marketplace-registry-api

FROM scratch
# CA certificates so review webhooks (REVIEW_WEBHOOK_URLS, https only) can
# verify the receiving server.
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /build/bin/marketplace-registry-api /
ENTRYPOINT ["/marketplace-registry-api"]
