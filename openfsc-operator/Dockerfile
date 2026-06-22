FROM golang:1.26.4-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY openfsc-operator/ ./openfsc-operator/
RUN CGO_ENABLED=0 go build -o /bin/openfsc-operator ./openfsc-operator/cmd

# No helm binary needed: charts are embedded and installed via the Helm SDK.
FROM scratch
COPY --from=builder /bin/openfsc-operator /
USER 65532:65532
ENTRYPOINT ["/openfsc-operator"]
