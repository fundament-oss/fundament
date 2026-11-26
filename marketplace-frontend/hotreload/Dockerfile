# Run `ng serve` on Node instead of Bun because V8 can be told to keep its heaps small
FROM oven/bun:1.4.3-alpine AS bun

FROM node:24.22-alpine
COPY --from=bun /usr/local/bin/bun /usr/local/bin/bun
WORKDIR /app

# Copy package files
COPY marketplace-frontend/package.json marketplace-frontend/bun.lock ./

# Install dependencies
RUN bun install --frozen-lockfile

# Copy source files
COPY marketplace-frontend/ .

# --optimize-for-size makes V8 grow slowly and return memory
# One build worker and the TypeScript compiler on the main thread prevent high idle memory usage
ENV NG_BUILD_MAX_WORKERS=1 \
    NG_BUILD_PARALLEL_TS=0

# Expose Angular dev server port (using 4000 to match the production container)
EXPOSE 4000

# Start development server with hot reload. It server-renders the storefront the
# same way the production server does; see marketplace-frontend/src/server.ts.
# NG_CONFIGURATION selects the audience, mirroring the production image's
# BUILD_SCRIPT: development (the storefront), registry or admin.
# Configure dev server to listen on 0.0.0.0:4000 for pod accessibility
ARG NG_CONFIGURATION=development
ENV NG_CONFIGURATION=$NG_CONFIGURATION
CMD ["sh", "-c", "exec node --optimize-for-size node_modules/@angular/cli/bin/ng.js serve --configuration \"$NG_CONFIGURATION\" --host 0.0.0.0 --port 4000"]
