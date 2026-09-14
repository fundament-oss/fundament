FROM golang:1.26.6-alpine

RUN go install github.com/air-verse/air@v1.65.0

WORKDIR /src

COPY go.mod go.sum ./

RUN go mod download

COPY plugin-controller/ ./plugin-controller/
COPY plugin-sdk/pluginruntime/ ./plugin-sdk/pluginruntime/
COPY common/ ./common/
# defclient speaks catalog.v1, which imports marketplace.v1.
COPY marketplace-catalog-api/pkg/proto/ ./marketplace-catalog-api/pkg/proto/
COPY marketplace-api/pkg/proto/ ./marketplace-api/pkg/proto/

CMD ["air", "-c", "plugin-controller/hotreload/.air.toml"]
