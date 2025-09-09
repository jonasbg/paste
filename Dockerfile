# Stage 1: Build WASM binaries and dependencies
FROM golang:1.23-alpine AS wasm-builder
WORKDIR /wasm
COPY wasm/ .

RUN apk add --no-cache wget
RUN wget https://github.com/tinygo-org/tinygo/releases/download/v0.39.0/tinygo0.39.0.linux-amd64.tar.gz \
    && tar -xzf tinygo0.39.0.linux-amd64.tar.gz \
    && mv tinygo /usr/local/

RUN GOOS=js GOARCH=wasm /usr/local/tinygo/bin/tinygo build -o encryption.wasm --no-debug wasm.go

RUN cp "/usr/local/tinygo/targets/wasm_exec.js" .

# Stage 2: Build the SvelteKit frontend with Bun
FROM node:23.9.0-alpine3.21 AS frontend-builder
WORKDIR /app/frontend

# Copy package.json and bun.lockb (if you're using Bun's lockfile)
COPY web/package*.json ./

# Install dependencies with clean npm cache and production only
RUN npm install --include=dev

# Copy the rest of the frontend code
COPY web .

# Build the SvelteKit app
RUN NODE_ENV=production npm run build

# Stage 3: Build the Go backend
FROM golang:1.23-alpine AS backend-builder
RUN apk add --update gcc musl-dev sqlite-dev --no-cache

WORKDIR /app/backend

# Copy go mod and sum files
COPY api/go.mod api/go.sum ./

# Download dependencies with verify
RUN go mod download && go mod verify

# Copy the source from the current directory to the working Directory inside the container
COPY api .

# Build with security flags and optimizations
RUN CGO_ENABLED=1 GOOS=linux go build -a \
    -ldflags='-w -s -linkmode external -extldflags "-static"' \
    -o paste .

# Stage 4: Final stage
FROM scratch

ENV GIN_MODE=release
ENV DATABASE_DIR=/uploads
ENV PASTE_RETENTION_DAYS=7
ENV LOGS_RETENTION_DAYS=180
ENV MAX_FILE_SIZE=2GB
ENV ID_SIZE=64
ENV KEY_SIZE=128

# Copy SSL certificates for HTTPS support
COPY --from=backend-builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Copy the binary and web files as user 101
COPY --from=backend-builder --chown=101:101 /app/backend/paste /paste
COPY --from=frontend-builder --chown=101:101 /app/frontend/build /web
COPY --from=wasm-builder --chown=101:101 /wasm/encryption.wasm /web/encryption.wasm
COPY --from=wasm-builder --chown=101:101 /wasm/wasm_exec.js /web/wasm_exec.js

# Define any necessary volumes
VOLUME ["/uploads"]
VOLUME ["/data"]

# Set user 101
USER 101

# Expose port 8080
EXPOSE 8080

# Run with explicit entrypoint and cmd
ENTRYPOINT ["/paste"]
CMD []
