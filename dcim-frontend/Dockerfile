# Build stage
# Use a real Node.js (>= v24.15) for `ng build`; Angular CLI 22 rejects Bun's
# Node-compat version (24.3.0). Bun is still used as the package manager.
FROM oven/bun:1.3.14-alpine AS bun

FROM node:24.17-alpine AS build
COPY --from=bun /usr/local/bin/bun /usr/local/bin/bun
WORKDIR /app

# Copy package files
COPY dcim-frontend/package.json dcim-frontend/bun.lock ./

# Install dependencies
RUN bun install --frozen-lockfile

# Copy proto files first (needed for generate-proto-version.js)
COPY dcim-api/pkg/proto/v1/ /dcim-api/pkg/proto/v1/

# Copy source files
COPY dcim-frontend/ .

# Build the application
RUN bun run build

# Production stage
FROM nginx:alpine
COPY --from=build /app/dist/dcim-console/browser /usr/share/nginx/html
COPY dcim-frontend/nginx.conf /etc/nginx/conf.d/default.conf.template

EXPOSE 80
CMD ["/bin/sh", "-c", "envsubst '${CONNECT_SRC}' < /etc/nginx/conf.d/default.conf.template > /etc/nginx/conf.d/default.conf && nginx -g 'daemon off;'"]
