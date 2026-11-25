FROM golang:1.27.4-alpine AS builder

WORKDIR /build

COPY go.mod go.sum ./

RUN go mod download

COPY gardener-extension-tenant-metrics/ ./gardener-extension-tenant-metrics/
COPY common/ ./common/

RUN CGO_ENABLED=0 go build -o /bin/gardener-extension-tenant-metrics ./gardener-extension-tenant-metrics/cmd

FROM scratch

COPY --from=builder /bin/gardener-extension-tenant-metrics /

ENTRYPOINT ["/gardener-extension-tenant-metrics"]
