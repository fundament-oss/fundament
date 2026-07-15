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

# Build the application
RUN bun run build

# Production stage
FROM nginx:alpine
COPY --from=build /app/dist/fundament-marketplace/browser /usr/share/nginx/html
COPY marketplace-frontend/nginx.conf /etc/nginx/conf.d/default.conf

EXPOSE 80
CMD ["nginx", "-g", "daemon off;"]
