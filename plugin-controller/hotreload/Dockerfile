FROM golang:1.26.5-alpine

RUN go install github.com/air-verse/air@v1.65.0

WORKDIR /src

COPY go.mod go.sum ./

RUN go mod download

COPY plugin-controller/ ./plugin-controller/
COPY plugin-sdk/pluginruntime/ ./plugin-sdk/pluginruntime/
COPY common/ ./common/
COPY organization-api/pkg/proto/ ./organization-api/pkg/proto/

CMD ["air", "-c", "plugin-controller/hotreload/.air.toml"]
