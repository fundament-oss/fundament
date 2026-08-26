FROM golang:1.26.6 AS build

WORKDIR /src
COPY go.mod go.sum ./
COPY plugin-sdk/go.mod plugin-sdk/go.sum ./plugin-sdk/
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /cert-manager-plugin ./plugins/cert-manager/

FROM alpine:3.21

RUN apk add --no-cache helm

COPY --from=build /cert-manager-plugin /usr/local/bin/cert-manager-plugin
COPY plugins/cert-manager/definition.yaml /app/definition.yaml

WORKDIR /app
ENTRYPOINT ["cert-manager-plugin"]
