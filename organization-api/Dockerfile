FROM golang:1.25.5-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY organization-api/ ./organization-api/
COPY common/ ./common/
# Generate proto hash for API versioning
RUN PROTO_HASH=$(find organization-api/pkg/proto/organization -name "*.proto" -type f -exec sha256sum {} + | sort | sha256sum | cut -d' ' -f1 | cut -c1-12) && \
    CGO_ENABLED=0 go build -ldflags="-X github.com/fundament-oss/fundament/organization-api/pkg/proto.APIVersion=${PROTO_HASH}" ./organization-api/cmd/fun-organization-api

FROM scratch
COPY --from=builder /build/fun-organization-api /
ENTRYPOINT ["/fun-organization-api"]
