FROM golang:1.26.6-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
COPY plugin-sdk/go.mod plugin-sdk/go.sum ./plugin-sdk/
RUN go mod download
COPY organization-api/ ./organization-api/
COPY plugin-sdk/ ./plugin-sdk/
COPY common/ ./common/
RUN CGO_ENABLED=0 go build ./organization-api/cmd/fun-organization-api

FROM scratch
COPY --from=builder /build/fun-organization-api /
ENTRYPOINT ["/fun-organization-api"]
