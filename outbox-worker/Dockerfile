FROM golang:1.25.5-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY outbox-worker/ ./outbox-worker/
COPY common/ ./common/
RUN CGO_ENABLED=0 go build ./outbox-worker/cmd/fun-outbox-worker

FROM scratch
COPY --from=builder /build/fun-outbox-worker /
ENTRYPOINT ["/fun-outbox-worker"]
