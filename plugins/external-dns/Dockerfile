FROM golang:1.26.1 AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /external-dns-plugin ./plugins/external-dns/

FROM alpine:3.21

RUN apk add --no-cache helm

COPY --from=build /external-dns-plugin /usr/local/bin/external-dns-plugin
COPY plugins/external-dns/definition.yaml /app/definition.yaml

WORKDIR /app
ENTRYPOINT ["external-dns-plugin"]
