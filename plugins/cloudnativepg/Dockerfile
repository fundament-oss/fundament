FROM golang:1.27.3 AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /cloudnativepg-plugin ./plugins/cloudnativepg/

FROM alpine:3.21

RUN apk add --no-cache helm

COPY --from=build /cloudnativepg-plugin /usr/local/bin/cloudnativepg-plugin
COPY plugins/cloudnativepg/definition.yaml /app/definition.yaml

# CloudNativePG's image catalog as of this build: the current, digest-pinned
# PostgreSQL image per major, republished weekly. ADD checks the URL on every
# build and reuses the cached layer only while the file is unchanged, and each
# plugin release brings its databases to the minors in it (cnpg/catalog.go).
ADD --chmod=644 https://raw.githubusercontent.com/cloudnative-pg/artifacts/main/image-catalogs/catalog-standard-bookworm.yaml /app/image-catalog.yaml

WORKDIR /app
ENTRYPOINT ["cloudnativepg-plugin"]
