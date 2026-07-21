# Use a real Node.js (>= v24.15) for `ng serve`; Angular CLI 22 rejects Bun's
# Node-compat version (24.3.0). Bun is still used as the package manager.
FROM oven/bun:1.3.14-alpine AS bun

FROM node:24.17-alpine
COPY --from=bun /usr/local/bin/bun /usr/local/bin/bun
WORKDIR /app

# Copy package files
COPY dcim-frontend/package.json dcim-frontend/bun.lock ./

# Install dependencies
RUN bun install --frozen-lockfile

# Copy source files
COPY dcim-frontend/ .

# Expose Angular dev server port (using 80 to match production service port)
EXPOSE 80

# Start development server with hot reload
CMD ["sh", "-c", "bun start --host 0.0.0.0 --port 80"]
