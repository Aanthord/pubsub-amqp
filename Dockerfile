# Stage 1: Build the Go binary
FROM golang:1.22-alpine AS builder

# Set the working directory inside the container
WORKDIR /app

# Install Swag CLI and other required packages
RUN apk add --no-cache git && \
    go install github.com/swaggo/swag/cmd/swag@latest

# Copy go.mod and go.sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod tidy

RUN go mod vendor

# Copy the entire project
COPY . .

# Generate Swagger documentation
RUN swag init -g cmd/app/main.go -o docs

# Build the Go application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o app ./cmd/app

# Stage 2: Create a smaller image for the final build
FROM alpine:latest

# Install ca-certificates for HTTPS support
RUN apk --no-cache add ca-certificates

# Set the working directory inside the final container
WORKDIR /root/

# Copy the binary and necessary files from the builder stage
COPY --from=builder /app/app .
COPY --from=builder /app/docs ./docs
COPY --from=builder /app/.env .

# Expose the port the application will run on
EXPOSE 8080

# Command to run the executable
CMD ["./app"]