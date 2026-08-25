FROM golang:1.26.6-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY marketplace-api/ ./marketplace-api/
COPY plugin-sdk/ ./plugin-sdk/
COPY common/ ./common/
RUN CGO_ENABLED=0 go build -o /build/bin/marketplace-catalog-api ./marketplace-api/cmd/fun-marketplace-catalog-api

FROM scratch
COPY --from=builder /build/bin/marketplace-catalog-api /
ENTRYPOINT ["/marketplace-catalog-api"]
