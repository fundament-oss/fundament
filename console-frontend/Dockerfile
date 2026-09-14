# Build stage
FROM oven/bun:1.4.0-alpine AS build
WORKDIR /app

# Copy package files
COPY console-frontend/package.json console-frontend/bun.lock ./

# Install dependencies
RUN bun install --frozen-lockfile

# Copy proto files first (needed for generate-proto-version.js)
COPY organization-api/pkg/proto/v1/ /organization-api/pkg/proto/v1/

# Copy source files
COPY console-frontend/ .

# The walkthrough frames the marketplace demo bundle, which has to be same-origin,
# so `build:demo` builds it from here and copies it into public/marketplace/. Only
# the demo build reads this; the default build ignores it.
COPY marketplace-frontend/ /marketplace-frontend/

# Build the application. `build:demo` produces the walkthrough bundle (main.demo.ts,
# in-memory mock data) that the console-demo deployment serves; it needs no config.json
# and makes no backend calls.
ARG BUILD_SCRIPT=build
RUN bun run "$BUILD_SCRIPT"

# Production stage
FROM nginx:alpine
COPY --from=build /app/dist/fundament-console/browser /usr/share/nginx/html
COPY console-frontend/nginx.conf /etc/nginx/conf.d/default.conf.template

EXPOSE 80
CMD ["/bin/sh", "-c", "envsubst '${CONNECT_SRC} ${FRAME_SRC}' < /etc/nginx/conf.d/default.conf.template > /etc/nginx/conf.d/default.conf && nginx -g 'daemon off;'"]
