FROM golang:1.26.3 AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /bin/gateway-api ./plugins/gateway-api

FROM alpine:3.22

RUN apk add --no-cache helm

COPY --from=build /bin/gateway-api /usr/local/bin/gateway-api
COPY plugins/gateway-api/definition.yaml /app/definition.yaml

WORKDIR /app
ENTRYPOINT ["gateway-api"]
