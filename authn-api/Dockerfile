FROM golang:1.25.5-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY authn-api/ ./authn-api/
COPY common/ ./common/
COPY db/dbversion/ ./db/dbversion/
RUN CGO_ENABLED=0 go build ./authn-api/cmd/fun-authn-api

FROM scratch
COPY --from=builder /build/fun-authn-api /
ENTRYPOINT ["/fun-authn-api"]
