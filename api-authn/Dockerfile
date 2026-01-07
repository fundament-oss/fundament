FROM golang:1.25.5-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY api-authn/ ./api-authn/
COPY pkg-common/ ./pkg-common/
RUN CGO_ENABLED=0 go build ./api-authn/cmd/fun-authn-api

FROM scratch
COPY --from=builder /build/fun-authn-api /
ENTRYPOINT ["/fun-authn-api"]
