# Stage 1: Build the Go binary
FROM golang:1.24-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates

# Set working directory
WORKDIR /app

# Copy modules files first for optimized dependency caching
COPY go.mod go.sum ./
RUN go mod download

# Copy the entire source tree
COPY . .

# Build the statically-linked high-performance production binary
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-s -w -X main.VERSION=docker-release" \
    -o ncmdump ./cmd/ncmdump

# Stage 2: Create a minimal production container
FROM alpine:3.21

# Install runtime security certificates (essential for downloading cover images from NetEase CDN)
RUN apk add --no-cache ca-certificates tzdata

# Create a non-privileged system user for container security compliance
RUN addgroup -S ncmgroup && adduser -S ncmuser -G ncmgroup

# Set execution workspace
WORKDIR /app

# Copy compiled binary from the builder stage
COPY --from=builder /app/ncmdump /app/ncmdump

# Copy configuration folder from the builder stage
COPY --from=builder /app/cmd/ncmdump/conf /app/conf

# Create volume mounting directories for source and target paths
RUN mkdir -p /data/source /data/target && \
    chown -R ncmuser:ncmgroup /app /data

# Switch to non-privileged user
USER ncmuser

# Declare host mounting volumes
VOLUME ["/data/source", "/data/target"]

# Define default entrypoint
ENTRYPOINT ["/app/ncmdump"]
