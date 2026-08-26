FROM golang:1.26.6-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
COPY plugin-sdk/go.mod plugin-sdk/go.sum ./plugin-sdk/
RUN go mod download
COPY authz-worker/ ./authz-worker/
COPY common/ ./common/
RUN CGO_ENABLED=0 go build ./authz-worker/cmd/fun-authz-worker

FROM scratch
COPY --from=builder /build/fun-authz-worker /
ENTRYPOINT ["/fun-authz-worker"]
