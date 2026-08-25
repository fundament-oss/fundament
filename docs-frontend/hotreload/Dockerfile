FROM oven/bun:1.4.0-alpine
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
# The script replaces each destination before copying, so it is safe to re-run
# over a tree that already has them.
COPY docs/ /tmp/docs/
RUN bun scripts/sync-docs.mjs --source /tmp/docs && rm -rf /tmp/docs

# Disable Astro telemetry
ENV ASTRO_TELEMETRY_DISABLED=1

# Expose Astro dev server port (using 80 to match production service port)
EXPOSE 80

# Start development server with hot reload
# Configure dev server to listen on 0.0.0.0:80 for pod accessibility
CMD ["sh", "-c", "bun start --host 0.0.0.0 --port 80"]
