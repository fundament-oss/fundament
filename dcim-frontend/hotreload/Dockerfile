FROM oven/bun:1-alpine
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
