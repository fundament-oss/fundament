FROM oven/bun:1-alpine
WORKDIR /app

# Copy package files
COPY docs-frontend/package.json docs-frontend/bun.lock ./

# Install dependencies
RUN bun install --frozen-lockfile

# Copy source files
COPY docs-frontend/ .

# Copy docs, FUNs, and ADRs into their respective content directories
# Serve assets statically to avoid Astro/Vite processing large SVGs. Note: cp instead of mv, since the docs/docs dir may already exist, resulting in docs/docs/docs after mv.
COPY docs/ /tmp/docs/
RUN mv /tmp/docs/funs ./src/content/docs/funs \
 && mv /tmp/docs/adr ./src/content/docs/adr \
 && cp -r /tmp/docs/assets public/assets \
 && cp -r /tmp/docs/. ./src/content/docs/docs && rm -rf /tmp/docs \
 && find ./src/content/docs/docs -name '*.md' -exec sed -i 's|(assets/|(/assets/|g' {} +

# Disable Astro telemetry
ENV ASTRO_TELEMETRY_DISABLED=1

# Expose Astro dev server port (using 80 to match production service port)
EXPOSE 80

# Start development server with hot reload
# Configure dev server to listen on 0.0.0.0:80 for pod accessibility
CMD ["sh", "-c", "bun start --host 0.0.0.0 --port 80"]
