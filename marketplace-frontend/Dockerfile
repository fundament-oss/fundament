# Build stage
# Use a real Node.js (>= v24.15) for `ng build`; Angular CLI 22 rejects Bun's
# Node-compat version (24.3.0). Bun is still used as the package manager.
FROM oven/bun:1.3.14-alpine AS bun

FROM node:24.17-alpine AS build
COPY --from=bun /usr/local/bin/bun /usr/local/bin/bun
WORKDIR /app

# Copy package files
COPY marketplace-frontend/package.json marketplace-frontend/bun.lock ./

# Install dependencies
RUN bun install --frozen-lockfile

# Copy source files
COPY marketplace-frontend/ .

# Build the application: a browser bundle plus the Node server that renders it
RUN bun run build

# Production stage
# The storefront is server-rendered, so this ships as a Node server rather than
# a static site. The server bundle is self-contained (esbuild inlines express
# and everything else), so no node_modules are needed here.
FROM node:24.17-alpine
WORKDIR /app
ENV NODE_ENV=production

COPY --from=build /app/dist/fundament-marketplace ./dist/fundament-marketplace

USER node
EXPOSE 4000
CMD ["node", "dist/fundament-marketplace/server/server.mjs"]
