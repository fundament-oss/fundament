FROM golang:1.26.6-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
COPY plugin-sdk/go.mod plugin-sdk/go.sum ./plugin-sdk/
RUN go mod download
COPY dcim-api/ ./dcim-api/
COPY common/ ./common/
RUN CGO_ENABLED=0 go build -o /build/bin/dcim-api ./dcim-api/cmd/dcim-api

FROM scratch
COPY --from=builder /build/bin/dcim-api /
ENTRYPOINT ["/dcim-api"]
