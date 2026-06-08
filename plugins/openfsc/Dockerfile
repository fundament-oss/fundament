FROM golang:1.26.4 AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /openfsc-plugin ./plugins/openfsc/

FROM alpine:3.21

# helm + git: the plugin clones the OpenFSC charts and installs them at runtime.
RUN apk add --no-cache helm git

COPY --from=build /openfsc-plugin /usr/local/bin/openfsc-plugin
COPY plugins/openfsc/definition.yaml /app/definition.yaml
# Vendored OpenFSC inway/outway charts (pinned v2.3.0); the operator helm-installs
# one release per Inway/Outway CR from these paths (no runtime fetch).
COPY plugins/openfsc/charts /charts

WORKDIR /app
ENTRYPOINT ["openfsc-plugin"]
