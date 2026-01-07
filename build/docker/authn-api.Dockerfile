FROM golang:1.25.5-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY pkg/ ./pkg/
COPY gen/ ./gen/
RUN CGO_ENABLED=0 go build -o authn-api ./cmd/authn-api

FROM scratch
COPY --from=builder /build/authn-api /
ENTRYPOINT ["/authn-api"]
