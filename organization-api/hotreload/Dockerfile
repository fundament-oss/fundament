FROM golang:1.26.0-alpine
RUN go install github.com/air-verse/air@latest
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY organization-api/ ./organization-api/
COPY common/ ./common/
CMD ["air", "-c", "organization-api/hotreload/.air.toml"]
