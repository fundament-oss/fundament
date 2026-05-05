FROM golang:1.26.2-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY dcim-api/ ./dcim-api/
COPY common/ ./common/
RUN CGO_ENABLED=0 go build -o /build/bin/dcim-api ./dcim-api/cmd/dcim-api

FROM scratch
COPY --from=builder /build/bin/dcim-api /
ENTRYPOINT ["/dcim-api"]
