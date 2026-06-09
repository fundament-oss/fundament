FROM golang:1.26.4 AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /openfsc-plugin ./plugins/openfsc/

FROM alpine:3.21

# helm: the plugin installs the vendored OpenFSC charts at runtime (no clone).
RUN apk add --no-cache helm

COPY --from=build /openfsc-plugin /usr/local/bin/openfsc-plugin
COPY plugins/openfsc/definition.yaml /app/definition.yaml
# Vendored OpenFSC charts, installed at runtime with no network fetch: the
# digilab umbrella (open-fsc-1.43.0.tgz) and the openfsc-directory helper chart
# for the install, plus the open-fsc-inway/outway charts the operator
# helm-installs one release per Inway/Outway CR.
COPY plugins/openfsc/charts /charts

WORKDIR /app
ENTRYPOINT ["openfsc-plugin"]
