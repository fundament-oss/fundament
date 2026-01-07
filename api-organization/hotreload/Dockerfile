FROM golang:1.25.5-alpine
RUN go install github.com/air-verse/air@latest
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY api-organization/ ./api-organization/
COPY pkg-common/ ./pkg-common/
CMD ["air", "-c", "api-organization/hotreload/.air.toml"]
