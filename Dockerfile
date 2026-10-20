# Stage 1: Build the WASM encryption module with TinyGo.
# Use the official TinyGo image rather than unpacking a release tarball into
# golang:alpine: newer TinyGo release binaries need glibc and fail on Alpine.
# TinyGo 0.43.0 bundles Go 1.27 and supports go.mod versions up to 1.27.
FROM tinygo/tinygo:0.43.0 AS wasm-builder
USER root

WORKDIR /wasm
COPY wasm/ .

# -opt=2 optimises for speed (~50% faster AES-GCM than the default -opt=z);
# -panic=trap drops panic-message code, which offsets most of the size cost.
# The module never panics on bad input (errors are returned to JS).
RUN tinygo build -o encryption.wasm -target wasm -no-debug -opt=2 -panic=trap wasm.go

RUN cp /usr/local/tinygo/targets/wasm_exec.js .

# Stage 2: Build the SvelteKit frontend with Bun
FROM node:24.22.0-alpine AS frontend-builder
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
FROM golang:1.27.3-alpine AS backend-builder

# Docker buildx automatically provides these
ARG TARGETOS
ARG TARGETARCH
ARG TARGETVARIANT

WORKDIR /app/backend

# Copy go mod and sum files
COPY api/go.mod api/go.sum ./

# Download dependencies with verify
RUN go mod download && go mod verify

# Copy the source from the current directory to the working Directory inside the container
COPY api .

# Build minimal binary with aggressive optimizations
# -trimpath: remove file system paths from binary
# -ldflags '-w -s': strip debug info and symbol table
# -tags netgo,osusergo: pure Go implementations of net and os/user
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} go build -a \
    -trimpath \
    -ldflags='-w -s -extldflags "-static"' \
    -tags netgo,osusergo \
    -o paste .

# Empty upload dir, copied with the runtime user's ownership: the scratch
# image has no mkdir, and a root-owned /uploads is not writable by user 101.
RUN mkdir -p /out/uploads

# Stage 4: Final stage
FROM scratch

ENV GIN_MODE=release
ENV UPLOAD_DIR=/uploads
ENV FILES_RETENTION_DAYS=7
ENV MAX_FILE_SIZE=2GB
ENV ID_SIZE=64
ENV KEY_SIZE=128

# Copy SSL certificates for HTTPS support
COPY --from=backend-builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Copy the binary and web files as user 101
COPY --from=backend-builder --chown=101:101 /app/backend/paste /paste
COPY --from=backend-builder --chown=101:101 /out/uploads /uploads
COPY --from=frontend-builder --chown=101:101 /app/frontend/build /web
COPY --from=wasm-builder --chown=101:101 /wasm/encryption.wasm /web/encryption.wasm
COPY --from=wasm-builder --chown=101:101 /wasm/wasm_exec.js /web/wasm_exec.js

# Define any necessary volumes
VOLUME ["/uploads"]

# Set user 101
USER 101

# Expose port 8080
EXPOSE 8080

# Run with explicit entrypoint and cmd
ENTRYPOINT ["/paste"]
CMD []
