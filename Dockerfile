# Stage 1: Builder
FROM golang:1.25-alpine AS builder

# git needed for repository initialization during build
RUN apk add --no-cache git

WORKDIR /app

# Download dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -o nonotes cmd/nonotes/main.go

# Stage 2: Runtime
FROM alpine:latest

# Install runtime dependencies: 
# - git (needed for history/blame functionality)
# - ca-certificates (for HTTPS)
# - tzdata (for time handling)
RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/nonotes .

# Copy web assets (templates and static files)
COPY web/ ./web/


# Expose the port
EXPOSE 8080

# Run the app
CMD ["./nonotes"]
