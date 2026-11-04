FROM golang:1.27.3-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY authz-api/ ./authz-api/
COPY common/ ./common/
RUN CGO_ENABLED=0 go build ./authz-api/cmd/fun-authz-api

FROM scratch
COPY --from=builder /build/fun-authz-api /
ENTRYPOINT ["/fun-authz-api"]
