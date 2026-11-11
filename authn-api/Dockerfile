FROM golang:1.27.4-alpine AS builder
RUN apk add --no-cache ca-certificates
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY authn-api/ ./authn-api/
COPY common/ ./common/
COPY plugin-proxy/ ./plugin-proxy/
RUN CGO_ENABLED=0 go build ./authn-api/cmd/fun-authn-api

FROM scratch
# CA certificates so OIDC discovery/JWKS over HTTPS works: with an external
# identity provider (authnApi.oidc) the binary makes outbound TLS calls to it.
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /build/fun-authn-api /
ENTRYPOINT ["/fun-authn-api"]
