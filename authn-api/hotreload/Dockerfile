FROM golang:1.26.2-alpine
RUN go install github.com/air-verse/air@v1.65.0
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY authn-api/ ./authn-api/
COPY common/ ./common/
CMD ["air", "-c", "authn-api/hotreload/.air.toml"]
