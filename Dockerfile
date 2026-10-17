# Stage 1: Build the Go binary
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Install git
RUN apk add --no-cache git

# Force Go to use the official proxy and direct fallback, bypassing direct git authentication issues
ENV GOPROXY=https://proxy.golang.org,direct
ENV GOSUMDB=sum.golang.org

# Copy go.mod first
COPY go.mod ./

# Copy the rest of the source code
COPY . .

# Download dependencies
RUN go mod tidy

# Build a statically linked Go binary
RUN CGO_ENABLED=0 GOOS=linux go build -o chatbot main.go

# Stage 2: Create a lightweight final image
FROM alpine:latest

WORKDIR /root/

# Copy the compiled binary from the builder stage
COPY --from=builder /app/chatbot .

# Expose the port your Fiber server listens on
EXPOSE 8080

# Command to run the application
CMD ["./chatbot"]