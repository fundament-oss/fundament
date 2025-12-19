FROM golang:1.25.5-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY appstore-api/ ./appstore-api/
RUN CGO_ENABLED=0 go build ./appstore-api/cmd/fun-appstore-api

FROM scratch
COPY --from=builder /build/fun-appstore-api /
ENTRYPOINT ["/fun-appstore-api"]
