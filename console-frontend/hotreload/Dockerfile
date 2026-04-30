FROM oven/bun:1-alpine
WORKDIR /app

# Copy package files
COPY console-frontend/package.json console-frontend/bun.lock ./

# Install dependencies
RUN bun install --frozen-lockfile

# Copy proto files (needed for generate-proto-version.js)
COPY organization-api/pkg/proto/v1/ /organization-api/pkg/proto/v1/

# Copy source files
COPY console-frontend/ .

# Expose Angular dev server port (using 80 to match production service port)
EXPOSE 80

# Start development server with hot reload
# Configure dev server to listen on 0.0.0.0:80 for pod accessibility
CMD ["sh", "-c", "bun start --host 0.0.0.0 --port 80"]
