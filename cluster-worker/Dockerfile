FROM golang:1.25.5-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY cluster-worker/ ./cluster-worker/
COPY common/ ./common/
RUN CGO_ENABLED=0 go build -o /bin/cluster-worker ./cluster-worker/cmd/cluster-worker

FROM scratch
COPY --from=builder /bin/cluster-worker /
ENTRYPOINT ["/cluster-worker"]
