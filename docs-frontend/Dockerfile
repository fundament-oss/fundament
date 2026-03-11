# Build stage
FROM oven/bun:1-alpine AS build
WORKDIR /app

# Copy package files
COPY docs-frontend/package.json docs-frontend/bun.lock ./

# Install dependencies
RUN bun install --frozen-lockfile

# Copy source files
COPY docs-frontend/ .

# Copy docs, FUNs, and ADRs into their respective content directories
# Serve assets statically to avoid Astro/Vite processing large SVGs
COPY docs/ /tmp/docs/
RUN mv /tmp/docs/funs ./src/content/docs/funs \
 && mv /tmp/docs/adr ./src/content/docs/adr \
 && cp -r /tmp/docs/assets public/assets \
 && mv /tmp/docs ./src/content/docs/docs \
 && find ./src/content/docs/docs -name '*.md' -exec sed -i 's|(assets/|(/assets/|g' {} +

# Build the application
RUN bun run build

# Production stage
FROM nginx:alpine
COPY --from=build /app/dist /usr/share/nginx/html
COPY docs-frontend/nginx.conf /etc/nginx/conf.d/default.conf

EXPOSE 80
CMD ["nginx", "-g", "daemon off;"]
