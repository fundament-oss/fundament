# Build stage
# Use a real Node.js (>= v24.15) for the Astro build; the tooling rejects Bun's
# Node-compat version (24.3.0). Bun is still used as the package manager.
FROM oven/bun:1.4.0-alpine AS bun

FROM node:24.17-alpine AS build
COPY --from=bun /usr/local/bin/bun /usr/local/bin/bun
WORKDIR /app

# Copy package files
COPY docs-frontend/package.json docs-frontend/bun.lock ./

# Install dependencies
RUN bun install --frozen-lockfile

# Copy source files
COPY docs-frontend/ .

# Copy docs, FUNs, ADRs and assets into their respective content directories and
# rewrite the links that are written for GitHub so they work on the site too.
# See docs-frontend/scripts/sync-docs.mjs for the link conventions this implies.
COPY docs/ /tmp/docs/
RUN node scripts/sync-docs.mjs --source /tmp/docs && rm -rf /tmp/docs

# Disable Astro telemetry
ENV ASTRO_TELEMETRY_DISABLED=1

# Build the application. src/links-validator.ts runs at the end of the build and
# fails it on any link that does not resolve — broken links are otherwise
# invisible in production, since nginx.conf falls back to `/index.html` and
# serves the home page with a 200.
RUN bun run build

# Run pagefind explicitly to ensure the search index is generated.
# Starlight's internal pagefind hook uses `npx` which may fail silently; `bun x`
# uses the already-installed package and fails the build loudly.
# Note: `bun x` rather than `bunx`, since only the `bun` binary is copied above.
RUN bun x pagefind --site dist

# Production stage
FROM nginx:alpine
COPY --from=build /app/dist /usr/share/nginx/html
COPY docs-frontend/nginx.conf /etc/nginx/conf.d/default.conf

EXPOSE 80
CMD ["nginx", "-g", "daemon off;"]
