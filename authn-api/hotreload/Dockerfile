FROM golang:1.25.5-alpine
RUN go install github.com/air-verse/air@latest
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY authn-api/ ./authn-api/
COPY common/ ./common/
COPY db/dbversion/ ./db/dbversion/
CMD ["air", "-c", "authn-api/hotreload/.air.toml"]
