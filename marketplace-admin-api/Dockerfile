FROM golang:1.26.6-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY marketplace-admin-api/ ./marketplace-admin-api/
# The API contract lives with neither service: admin.v1 imports marketplace.v1.
COPY marketplace-api/ ./marketplace-api/
# GetPluginVersion derives capabilities from the pinned manifest with pluginruntime.
COPY plugin-sdk/pluginruntime/ ./plugin-sdk/pluginruntime/
COPY common/ ./common/
RUN CGO_ENABLED=0 go build -o /build/bin/marketplace-admin-api ./marketplace-admin-api/cmd/fun-marketplace-admin-api

FROM scratch
COPY --from=builder /build/bin/marketplace-admin-api /
ENTRYPOINT ["/marketplace-admin-api"]
