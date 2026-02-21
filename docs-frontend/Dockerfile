# Build stage
FROM oven/bun:1-alpine AS build
WORKDIR /app

# Copy package files
COPY docs-frontend/package.json docs-frontend/bun.lock ./

# Install dependencies
RUN bun install --frozen-lockfile

# Copy source files
COPY docs-frontend/ .

# Copy FUNs from the main repo
COPY funs/ ./src/content/docs/funs/

# Copy diagram SVGs from docs/assets to public/img for static serving
COPY docs/assets/infrastructure-stack.drawio.svg ./public/img/infrastructure-stack.drawio.svg
COPY docs/assets/resource-model.drawio.svg ./public/img/resource-model.drawio.svg

# Build the application
RUN bun run build

# Production stage
FROM nginx:alpine
COPY --from=build /app/dist /usr/share/nginx/html
COPY docs-frontend/nginx.conf /etc/nginx/conf.d/default.conf

EXPOSE 80
CMD ["nginx", "-g", "daemon off;"]
