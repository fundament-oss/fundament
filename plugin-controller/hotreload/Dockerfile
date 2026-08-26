FROM golang:1.26.6-alpine

RUN go install github.com/air-verse/air@v1.65.0

WORKDIR /src

COPY go.mod go.sum ./
COPY plugin-sdk/go.mod plugin-sdk/go.sum ./plugin-sdk/

RUN go mod download

COPY plugin-controller/ ./plugin-controller/
COPY plugin-sdk/ ./plugin-sdk/
COPY common/ ./common/
COPY organization-api/pkg/proto/ ./organization-api/pkg/proto/

CMD ["air", "-c", "plugin-controller/hotreload/.air.toml"]
