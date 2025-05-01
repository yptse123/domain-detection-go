# Build stage
FROM golang:1.24.2-alpine AS builder

WORKDIR /app

# Install required packages
RUN apk add --no-cache git

# Copy go.mod and go.sum files first to leverage Docker cache
COPY go.mod go.sum ./
RUN go mod download

# Copy the source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o domain-detection-api ./cmd/api/main.go

# Runtime stage
FROM alpine:3.19

WORKDIR /app

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata

# Copy the binary from builder stage
COPY --from=builder /app/domain-detection-api .

# Set environment variables
ENV GIN_MODE=release

EXPOSE 8080

# Run the application
CMD ["./domain-detection-api"]