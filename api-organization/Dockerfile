FROM golang:1.25.5-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY api-organization/ ./api-organization/
COPY pkg-common/ ./pkg-common/
RUN CGO_ENABLED=0 go build ./api-organization/cmd/fun-organization-api

FROM scratch
COPY --from=builder /build/fun-organization-api /
ENTRYPOINT ["/fun-organization-api"]
