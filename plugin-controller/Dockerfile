FROM golang:1.26.1-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY plugin-controller/ ./plugin-controller/
COPY plugin-sdk/pluginruntime/ ./plugin-sdk/pluginruntime/
COPY common/ ./common/
RUN CGO_ENABLED=0 go build -o /bin/plugin-controller ./plugin-controller/cmd

FROM scratch
COPY --from=builder /bin/plugin-controller /
ENTRYPOINT ["/plugin-controller"]
