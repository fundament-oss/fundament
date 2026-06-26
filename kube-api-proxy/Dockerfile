FROM golang:1.26.4-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY kube-api-proxy/ ./kube-api-proxy/
COPY common/ ./common/
RUN CGO_ENABLED=0 go build -o /bin/fun-kube-api-proxy ./kube-api-proxy/cmd/fun-kube-api-proxy

COPY plugins/ ./plugins/
RUN mkdir -p /assets/plugins && \
    for dir in ./plugins/*/console; do \
      [ -d "$dir" ] || continue; \
      name=$(basename "$(dirname "$dir")"); \
      mkdir -p "/assets/plugins/$name"; \
      cp -r "$dir" "/assets/plugins/$name/console"; \
    done

FROM scratch
COPY --from=builder /bin/fun-kube-api-proxy /
COPY --from=builder /assets/plugins /plugins
ENTRYPOINT ["/fun-kube-api-proxy"]
