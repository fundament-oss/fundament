FROM golang:1.26.6 AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /ceph-rook-plugin ./plugins/storage/ceph-rook/

FROM alpine:3.21

RUN apk add --no-cache helm

COPY --from=build /ceph-rook-plugin /usr/local/bin/ceph-rook-plugin
COPY plugins/storage/ceph-rook/definition.yaml /app/definition.yaml

WORKDIR /app
ENTRYPOINT ["ceph-rook-plugin"]
