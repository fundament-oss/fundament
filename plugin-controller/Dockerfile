FROM golang:1.26.6-alpine AS builder

WORKDIR /build

COPY go.mod go.sum ./
COPY plugin-sdk/go.mod plugin-sdk/go.sum ./plugin-sdk/

RUN go mod download

COPY plugin-controller/ ./plugin-controller/
COPY plugin-sdk/ ./plugin-sdk/
COPY common/ ./common/
COPY organization-api/pkg/proto/ ./organization-api/pkg/proto/

RUN CGO_ENABLED=0 go build -o /bin/plugin-controller ./plugin-controller/cmd

FROM scratch

COPY --from=builder /bin/plugin-controller /

ENTRYPOINT ["/plugin-controller"]
