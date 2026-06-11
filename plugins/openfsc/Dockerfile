FROM golang:1.26.4 AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /openfsc-plugin ./plugins/openfsc/

FROM alpine:3.21

# helm: the plugin installs the prerequisite charts (cert-manager,
# CloudNativePG) and the vendored openfsc-operator chart at runtime.
RUN apk add --no-cache helm

COPY --from=build /openfsc-plugin /usr/local/bin/openfsc-plugin
COPY plugins/openfsc/definition.yaml /app/definition.yaml
# The openfsc-operator chart (CRDs + Deployment + scoped RBAC), installed
# offline by the plugin; the operator image itself comes from FUNP_OPERATOR_IMAGE.
COPY openfsc-operator/chart /operator-chart

WORKDIR /app
ENTRYPOINT ["openfsc-plugin"]
