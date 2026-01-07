FROM golang:1.25.5-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY api-appstore/ ./api-appstore/
RUN CGO_ENABLED=0 go build ./api-appstore/cmd/fun-appstore-api

FROM scratch
COPY --from=builder /build/fun-appstore-api /
ENTRYPOINT ["/fun-appstore-api"]
