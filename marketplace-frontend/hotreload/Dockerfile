# Use a real Node.js (>= v24.15) for `ng serve`; Angular CLI 22 rejects Bun's
# Node-compat version (24.3.0). Bun is still used as the package manager.
FROM oven/bun:1.3.14-alpine AS bun

FROM node:24.17-alpine
COPY --from=bun /usr/local/bin/bun /usr/local/bin/bun
WORKDIR /app

# Copy package files
COPY marketplace-frontend/package.json marketplace-frontend/bun.lock ./

# Install dependencies
RUN bun install --frozen-lockfile

# Copy source files
COPY marketplace-frontend/ .

# Expose Angular dev server port (using 4000 to match the production container)
EXPOSE 4000

# Start development server with hot reload. It server-renders the storefront the
# same way the production server does; see marketplace-frontend/src/server.ts.
# START_SCRIPT selects the audience, mirroring the production image's
# BUILD_SCRIPT: start, start:registry or start:admin.
# Configure dev server to listen on 0.0.0.0:4000 for pod accessibility
ARG START_SCRIPT=start
ENV START_SCRIPT=$START_SCRIPT
CMD ["sh", "-c", "bun run \"$START_SCRIPT\" --host 0.0.0.0 --port 4000"]
