FROM golang:1.26.4-alpine AS builder
RUN apk add --no-cache ca-certificates
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY dcim-authn-api/ ./dcim-authn-api/
COPY common/ ./common/
RUN CGO_ENABLED=0 go build -o /build/bin/dcim-authn-api ./dcim-authn-api/cmd/dcim-authn-api

FROM scratch
# CA certificates so OIDC discovery/JWKS over HTTPS works (the binary makes
# outbound TLS calls to the identity provider).
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /build/bin/dcim-authn-api /
ENTRYPOINT ["/dcim-authn-api"]
