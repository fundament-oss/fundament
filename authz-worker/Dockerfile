FROM golang:1.25.5-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY authz-worker/ ./authz-worker/
COPY common/ ./common/
RUN CGO_ENABLED=0 go build ./authz-worker/cmd/fun-authz-worker

FROM scratch
COPY --from=builder /build/fun-authz-worker /
ENTRYPOINT ["/fun-authz-worker"]
