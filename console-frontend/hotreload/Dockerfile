FROM oven/bun:1-alpine
WORKDIR /app

# Copy package files
COPY console-frontend/package.json console-frontend/bun.lock console-frontend/bunfig.toml ./

# Install dependencies (token for @minbzk/* packages on npm.pkg.github.com, not stored in image)
RUN --mount=type=secret,id=github_token \
    GITHUB_TOKEN=$(cat /run/secrets/github_token) bun install --frozen-lockfile

# Copy proto files (needed for generate-proto-version.js)
COPY organization-api/pkg/proto/v1/ /organization-api/pkg/proto/v1/

# Copy source files
COPY console-frontend/ .

# Expose Angular dev server port (using 80 to match production service port)
EXPOSE 80

# Start development server with hot reload
# Configure dev server to listen on 0.0.0.0:80 for pod accessibility
CMD ["sh", "-c", "bun start --host 0.0.0.0 --port 80"]
