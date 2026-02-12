FROM alpine:3.23.3

ARG FGA_VERSION=0.7.8

RUN apk add --no-cache \
    curl \
    jq \
    bash \
    kubectl \
    && curl -sL "https://github.com/openfga/cli/releases/download/v${FGA_VERSION}/fga_${FGA_VERSION}_linux_amd64.tar.gz" | tar xz -C /usr/local/bin fga \
    && chmod +x /usr/local/bin/fga

ENTRYPOINT ["/bin/bash"]
