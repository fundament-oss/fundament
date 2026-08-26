FROM golang:1.26.6-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY openfga/ ./openfga/
RUN CGO_ENABLED=0 go build ./openfga/setup/cmd/fun-openfga-setup

FROM scratch
COPY --from=builder /build/fun-openfga-setup /
ENTRYPOINT ["/fun-openfga-setup"]
