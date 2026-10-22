FROM golang:1.27.3-alpine

ARG PLUGIN_PATH

# Plugin binaries shell out to helm at runtime; match the prod image.
RUN apk add --no-cache helm
RUN go install github.com/air-verse/air@v1.65.0

# skaffold sync extracts tar into /src as uid 65532; the pod runs as that uid too.
# Owned via --chown and USER rather than a chown -R layer that would duplicate /src.
RUN mkdir /src && chown 65532:65532 /src
WORKDIR /src
COPY --chown=65532:65532 go.mod go.sum ./
RUN go mod download
COPY --chown=65532:65532 plugin-sdk/ ./plugin-sdk/
COPY --chown=65532:65532 common/ ./common/
COPY --chown=65532:65532 plugins/ ./plugins/

USER 65532
RUN sed "s|@PATH@|${PLUGIN_PATH}|g" plugins/hotreload/.air.toml.tmpl > .air.toml

ENV GOCACHE=/src/.gocache
# Bake $GOCACHE at build time so air's first in-container build is a cache hit
# (compile happens here, throttled by skaffold concurrency — not as a runtime storm).
RUN go build -o /dev/null ./plugins/${PLUGIN_PATH}/

CMD ["air", "-c", ".air.toml"]
