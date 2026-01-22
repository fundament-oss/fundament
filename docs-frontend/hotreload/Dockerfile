FROM oven/bun:1-alpine
WORKDIR /app

# Copy package files
COPY docs-frontend/package.json docs-frontend/bun.lock ./

# Install dependencies
RUN bun install --frozen-lockfile

# Copy source files
COPY docs-frontend/ .

# Copy FUNs from the main repo
COPY funs/ ./src/content/docs/funs/

# Copy docs from the main repo and create a docs subdirectory
RUN mkdir -p ./src/content/docs/docs
COPY docs/ ./src/content/docs/docs/

# Expose Astro dev server port (using 80 to match production service port)
EXPOSE 80

# Start development server with hot reload
# Configure dev server to listen on 0.0.0.0:80 for pod accessibility
CMD ["sh", "-c", "bun start --host 0.0.0.0 --port 80"]
