# Stage 1: Build the application
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Copy go module files
COPY go.mod go.sum ./
# Download dependencies
RUN go mod download

# Copy the source code
COPY . .

# Build the application as a static binary
# Ensure the output path matches the entrypoint in the final stage
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o /app/geminiproxy ./cmd/geminiproxy

# Stage 2: Create the final lightweight image
FROM alpine:latest

WORKDIR /app

# Copy the static binary from the builder stage
COPY --from=builder /app/geminiproxy /app/geminiproxy

# Copy the keys file from the build context
# IMPORTANT: Ensure gemini.keys exists in the directory where you run 'docker build'
#            and manage this file securely (e.g., add to .gitignore).
COPY gemini.keys /app/gemini.keys

# Expose the port the application runs on
EXPOSE 8081

# Set the entrypoint to run the binary
ENTRYPOINT ["/app/geminiproxy"]
