FROM golang:1.25.5-alpine
RUN go install github.com/air-verse/air@latest
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY api-authn/ ./api-authn/
COPY pkg-common/ ./pkg-common/
CMD ["air", "-c", "api-authn/hotreload/.air.toml"]
