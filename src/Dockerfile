# Stage 2: Build the Go backend
FROM golang:1.23-alpine AS backend-builder
RUN apk add --update gcc musl-dev --no-cache
WORKDIR /app

# Copy go mod and sum files
COPY go.mod *.sum ./

# Download dependencies with verify
RUN go mod download && go mod verify

# Copy the source from the current directory to the working Directory inside the container
COPY . .

# Build with security flags and optimizations
RUN CGO_ENABLED=1 GOOS=linux go build -a \
    -ldflags='-w -s -linkmode external -extldflags "-static"' \
    -o nhn-share .

# Stage 3: Final stage
FROM scratch

ENV GIN_MODE=release

# Copy SSL certificates for HTTPS support
COPY --from=backend-builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Copy the binary and web files as user 101
COPY --from=backend-builder --chown=101:101 /app/nhn-share /nhn-share
COPY --chown=101:101 index.html /index.html

# Define any necessary volumes
VOLUME ["/upload"]

# Set user 101
USER 101

# Expose port 8080
EXPOSE 8080

# Run with explicit entrypoint and cmd
ENTRYPOINT ["/nhn-share"]
CMD []
