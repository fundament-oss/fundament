#!/bin/sh
exec /go/bin/dlv exec \
  --headless \
  --listen=:2345 \
  --api-version=2 \
  --accept-multiclient \
  --continue \
  -- /src/tmp/fun-kube-api-proxy
