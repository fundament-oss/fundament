FROM golang:1.27.1 AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /cloudnativepg-plugin ./plugins/cloudnativepg/

FROM alpine:3.21

RUN apk add --no-cache helm

COPY --from=build /cloudnativepg-plugin /usr/local/bin/cloudnativepg-plugin
COPY plugins/cloudnativepg/definition.yaml /app/definition.yaml

WORKDIR /app
ENTRYPOINT ["cloudnativepg-plugin"]
