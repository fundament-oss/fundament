FROM node:24-alpine
WORKDIR /app

# Install pnpm
RUN corepack enable && corepack prepare pnpm@latest --activate

# Copy package files
COPY console-frontend/package.json console-frontend/pnpm-lock.yaml ./

# Install dependencies
RUN pnpm install --frozen-lockfile

# Copy proto files (needed for generate-proto-version.js)
COPY organization-api/pkg/proto/v1/ /organization-api/pkg/proto/v1/

# Copy source files
COPY console-frontend/ .

# Expose Angular dev server port (using 80 to match production service port)
EXPOSE 80

# Start development server with hot reload
# Configure dev server to listen on 0.0.0.0:80 for pod accessibility
ARG APP_CONFIGURATION
CMD ["sh", "-c", "pnpm start --host 0.0.0.0 --port 80 --configuration $APP_CONFIGURATION"]
