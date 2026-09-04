FROM golang:1.26.6-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY openfga/ ./openfga/
COPY common/ ./common/
RUN CGO_ENABLED=0 go build ./openfga/bootstrap/cmd/fun-openfga-bootstrap

FROM scratch
COPY --from=builder /build/fun-openfga-bootstrap /
ENTRYPOINT ["/fun-openfga-bootstrap"]
