# Build stage
FROM oven/bun:1-alpine AS build
WORKDIR /app

# Copy package files
COPY console-frontend/package.json console-frontend/bun.lock console-frontend/bunfig.toml ./

# Install dependencies (token for @minbzk/* packages on npm.pkg.github.com, not stored in image)
RUN --mount=type=secret,id=github_token \
    GITHUB_TOKEN=$(cat /run/secrets/github_token) bun install --frozen-lockfile

# Copy proto files first (needed for generate-proto-version.js)
COPY organization-api/pkg/proto/v1/ /organization-api/pkg/proto/v1/

# Copy source files
COPY console-frontend/ .

# Build the application
RUN bun run build

# Production stage
FROM nginx:alpine
COPY --from=build /app/dist/fundament-console/browser /usr/share/nginx/html
COPY console-frontend/nginx.conf /etc/nginx/conf.d/default.conf.template

EXPOSE 80
CMD ["/bin/sh", "-c", "envsubst '${CONNECT_SRC}' < /etc/nginx/conf.d/default.conf.template > /etc/nginx/conf.d/default.conf && nginx -g 'daemon off;'"]
