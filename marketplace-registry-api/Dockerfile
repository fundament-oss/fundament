FROM golang:1.26.6-alpine AS builder
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
COPY --from=builder /build/bin/marketplace-registry-api /
ENTRYPOINT ["/marketplace-registry-api"]
