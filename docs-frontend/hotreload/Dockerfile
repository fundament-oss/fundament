FROM node:24-alpine
WORKDIR /app

# Install pnpm
RUN corepack enable && corepack prepare pnpm@latest --activate

# Copy package files
COPY docs-frontend/package.json docs-frontend/pnpm-lock.yaml ./

# Install dependencies
RUN pnpm install --frozen-lockfile

# Copy source files
COPY docs-frontend/ .

# Copy FUNs from the main repo
COPY funs/ ./src/content/docs/funs/

# Copy docs from the main repo and create a docs subdirectory
RUN mkdir -p ./src/content/docs/docs
COPY docs/ ./src/content/docs/docs/

# Add Starlight frontmatter to markdown files in the docs directory
RUN for file in ./src/content/docs/docs/*.md; do \
    if [ -f "$file" ]; then \
      filename=$(basename "$file" .md); \
      title=$(echo "$filename" | sed 's/-/ /g' | awk '{for(i=1;i<=NF;i++) $i=toupper(substr($i,1,1)) tolower(substr($i,2))}1'); \
      temp_file=$(mktemp); \
      echo "---" > "$temp_file"; \
      echo "title: $title" >> "$temp_file"; \
      echo "---" >> "$temp_file"; \
      echo "" >> "$temp_file"; \
      tail -n +2 "$file" >> "$temp_file"; \
      mv "$temp_file" "$file"; \
    fi \
  done

# Expose Astro dev server port (using 80 to match production service port)
EXPOSE 80

# Start development server with hot reload
# Configure dev server to listen on 0.0.0.0:80 for pod accessibility
CMD ["sh", "-c", "pnpm start --host 0.0.0.0 --port 80"]
