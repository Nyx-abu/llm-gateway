# Stage 1: Build
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Copy dependency manifest
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY cmd/mock-server/ ./cmd/mock-server/

# Build static binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o mock-server ./cmd/mock-server/main.go

# Stage 2: Final image
FROM alpine:3.19

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/mock-server .

# Expose HTTP port
EXPOSE 8081

# Run the mock server
CMD ["./mock-server"]
