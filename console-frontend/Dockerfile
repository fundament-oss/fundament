# Build stage
FROM node:24-alpine AS build
WORKDIR /app

# Install pnpm
RUN corepack enable && corepack prepare pnpm@latest --activate

# Copy package files
COPY console-frontend/package.json console-frontend/pnpm-lock.yaml ./

# Install dependencies
RUN pnpm install --frozen-lockfile

# Copy proto files first (needed for generate-proto-version.js)
COPY organization-api/pkg/proto/v1/ /organization-api/pkg/proto/v1/

# Copy source files
COPY console-frontend/ .

# Build the application with environment variables
ARG APP_CONFIGURATION
RUN ["sh", "-c", "pnpm build --configuration $APP_CONFIGURATION"]

# Production stage
FROM nginx:alpine
COPY --from=build /app/dist/fundament-console/browser /usr/share/nginx/html
COPY console-frontend/nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 80
CMD ["nginx", "-g", "daemon off;"]
