# Stage 1: Build
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Copy dependency manifest
COPY go.mod go.sum ./

# Download dependencies (this will download go-control-plane and grpc)
RUN go mod download

# Copy source code
COPY cmd/sidecar/ ./cmd/sidecar/
COPY pkg/extproc/ ./pkg/extproc/

# Build static binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o sidecar ./cmd/sidecar/main.go

# Stage 2: Final image
FROM alpine:3.19

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/sidecar .

# Expose gRPC port
EXPOSE 50051

# Run the sidecar
CMD ["./sidecar"]
