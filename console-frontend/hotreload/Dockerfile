# Run `ng serve` on Node instead of Bun because V8 can be told to keep its heaps small
FROM oven/bun:1.4.3-alpine AS bun

FROM node:24.22-alpine
COPY --from=bun /usr/local/bin/bun /usr/local/bin/bun
WORKDIR /app

# Copy package files
COPY console-frontend/package.json console-frontend/bun.lock ./

# Install dependencies
RUN bun install --frozen-lockfile

# Copy proto files (needed for generate-proto-version.js)
COPY organization-api/pkg/proto/v1/ /organization-api/pkg/proto/v1/

# Copy source files
COPY console-frontend/ .

# --optimize-for-size makes V8 grow slowly and return memory
# One build worker and the TypeScript compiler on the main thread prevent high idle memory usage
ENV NG_BUILD_MAX_WORKERS=1 \
    NG_BUILD_PARALLEL_TS=0

# Expose Angular dev server port (using 80 to match production service port)
EXPOSE 80

# Start development server with hot reload
# Configure dev server to listen on 0.0.0.0:80 for pod accessibility
CMD ["sh", "-c", "bun run prestart && exec node --optimize-for-size node_modules/@angular/cli/bin/ng.js serve --host 0.0.0.0 --port 80"]
