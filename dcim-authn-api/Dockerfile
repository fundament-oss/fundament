FROM golang:1.26.3-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY dcim-authn-api/ ./dcim-authn-api/
COPY common/ ./common/
RUN CGO_ENABLED=0 go build -o /build/bin/dcim-authn-api ./dcim-authn-api/cmd/dcim-authn-api

FROM scratch
COPY --from=builder /build/bin/dcim-authn-api /
ENTRYPOINT ["/dcim-authn-api"]
