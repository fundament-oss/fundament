FROM golang:1.25.5-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY organization-api/ ./organization-api/
RUN CGO_ENABLED=0 go build ./organization-api/cmd/fun-organization-api

FROM scratch
COPY --from=builder /build/fun-organization-api /
ENTRYPOINT ["/fun-organization-api"]
