FROM golang:1.25.5-alpine
RUN go install github.com/air-verse/air@latest
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY api-appstore/ ./api-appstore/
CMD ["air", "-c", "api-appstore/hotreload/.air.toml"]
