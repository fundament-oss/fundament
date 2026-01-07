FROM golang:1.25.5-alpine
RUN go install github.com/air-verse/air@latest
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY pkg/ ./pkg/
COPY gen/ ./gen/
COPY build/hotreload/appstore-api/.air.toml ./.air.toml
CMD ["air", "-c", ".air.toml"]
