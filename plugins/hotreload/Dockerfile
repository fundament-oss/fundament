FROM golang:1.27.3-alpine

ARG PLUGIN_PATH
ENV PLUGIN_PATH=${PLUGIN_PATH}

# Plugin binaries shell out to helm at runtime; match the prod image.
RUN apk add --no-cache helm
RUN go install github.com/air-verse/air@v1.65.0

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY plugin-sdk/ ./plugin-sdk/
COPY common/ ./common/
COPY plugins/ ./plugins/

ENV GOCACHE=/src/.gocache
# Bake $GOCACHE at build time so air's first in-container build is a cache hit
# (compile happens here, throttled by skaffold concurrency — not as a runtime storm).
RUN go build -o /dev/null ./plugins/${PLUGIN_PATH}/

# skaffold sync extracts tar into /src as uid 65532; the pod runs as that uid too.
RUN chown -R 65532:65532 /src

CMD ["sh", "-c", "air -c plugins/${PLUGIN_PATH}/hotreload/.air.toml"]
