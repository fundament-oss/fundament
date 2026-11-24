FROM golang:1.27.4 AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /zot-plugin ./plugins/zot/

FROM alpine:3.24

RUN apk add --no-cache helm

COPY --from=build /zot-plugin /usr/local/bin/zot-plugin
COPY plugins/zot/definition.yaml /app/definition.yaml

WORKDIR /app
ENTRYPOINT ["zot-plugin"]
