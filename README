# PubSub AMQP Service

## Overview

This project implements a robust Publish-Subscribe (PubSub) service using AMQP (Advanced Message Queuing Protocol). It's designed to handle high-throughput message processing with features like distributed tracing, metrics collection, and data persistence across various storage systems.

## Table of Contents

1. [Features](#features)
2. [Architecture](#architecture)
3. [Prerequisites](#prerequisites)
4. [Installation](#installation)
5. [Configuration](#configuration)
6. [Usage](#usage)
7. [API Documentation](#api-documentation)
8. [Development](#development)
9. [Testing](#testing)
10. [Deployment](#deployment)
11. [Monitoring and Logging](#monitoring-and-logging)
12. [Troubleshooting](#troubleshooting)
13. [Contributing](#contributing)
14. [License](#license)

## Features

- AMQP-based publish-subscribe messaging
- RESTful API for publishing and subscribing to topics
- Distributed tracing with Jaeger
- Metrics collection with Prometheus
- Data persistence with:
  - Amazon S3 for large message payloads
  - Amazon Redshift for analytics
  - Neo4j for graph-based data relationships
  - Local file storage for quick access
- UUID generation service
- Search functionality across persisted data
- Configurable CORS settings
- Swagger API documentation
- Dockerized deployment

## Architecture

The service is built using a modular architecture with the following key components:

- `cmd/app`: Contains the main application entry point
- `internal/`:
  - `amqp`: AMQP service implementation
  - `config`: Application configuration management
  - `handlers`: HTTP request handlers
  - `metrics`: Prometheus metrics setup
  - `models`: Data models
  - `search`: Search service implementation
  - `storage`: Data persistence implementations (S3, Redshift, Neo4j, File)
  - `tracing`: Distributed tracing setup with Jaeger
  - `uuid`: UUID generation service

The application uses the following external services:
- AMQP server (e.g., RabbitMQ) for message queuing
- Amazon S3 for large message storage
- Amazon Redshift for data warehousing
- Neo4j for graph database storage
- Jaeger for distributed tracing
- Prometheus for metrics collection

## Prerequisites

- Go 1.20 or later
- Docker and Docker Compose
- Access to AMQP server (e.g., RabbitMQ)
- AWS account with S3 and Redshift access
- Neo4j database
- Jaeger server for distributed tracing
- Prometheus server for metrics collection

## Installation

1. Clone the repository:
   ```
   git clone https://github.com/aanthord/pubsub-amqp.git
   cd pubsub-amqp
   ```

2. Install dependencies:
   ```
   go mod download
   ```

3. Build the application:
   ```
   go build -o app ./cmd/app
   ```

## Configuration

1. Copy the `.env.example` file to `.env`:
   ```
   cp .env.example .env
   ```

2. Edit the `.env` file and set the appropriate values for your environment.

3. The application uses the following environment variables:
   - `PORT`: Server port (default: 8080)
   - `AWS_REGION`: AWS region for S3 and Redshift
   - `AWS_S3_BUCKET`: S3 bucket name
   - `NEO4J_URI`: Neo4j connection URI
   - `NEO4J_USERNAME`: Neo4j username
   - `NEO4J_PASSWORD`: Neo4j password
   - `REDSHIFT_CONN_STRING`: Redshift connection string
   - `FILE_STORAGE_PATH`: Local file storage path
   - `AMQP_URL`: AMQP server URL
   - `S3_OFFLOAD_LIMIT`: Message size limit for S3 offloading
   - `JAEGER_AGENT_HOST`: Jaeger agent host
   - `JAEGER_AGENT_PORT`: Jaeger agent port
   - `LOG_LEVEL`: Logging level
   - `METRICS_PORT`: Prometheus metrics port
   - `CORS_ALLOWED_ORIGINS`: Comma-separated list of allowed origins for CORS
   - `CORS_ALLOWED_METHODS`: Comma-separated list of allowed HTTP methods for CORS
   - `CORS_ALLOWED_HEADERS`: Comma-separated list of allowed HTTP headers for CORS
   - `CORS_ALLOW_CREDENTIALS`: Whether to allow credentials for CORS requests
   - `CORS_MAX_AGE`: Max age for CORS preflight requests

## Usage

1. Start the application:
   ```
   ./app
   ```

2. The server will start on the configured port (default: 8080).

3. Use the provided API endpoints to publish and subscribe to topics.

## API Documentation

Swagger documentation is available at `/swagger/index.html` when the server is running.

Key endpoints:
- `POST /api/v1/publish/{topic}`: Publish a message to a topic
- `GET /api/v1/subscribe/{topic}`: Subscribe to messages from a topic
- `GET /api/v1/uuid`: Generate a new UUID
- `GET /api/v1/search`: Search for messages

## Development

1. Install Swag for Swagger documentation generation:
   ```
   go install github.com/swaggo/swag/cmd/swag@latest
   ```

2. Generate Swagger documentation:
   ```
   swag init -g cmd/app/main.go -o docs
   ```

3. Run the application in development mode:
   ```
   go run cmd/app/main.go
   ```

## Testing

Run the tests with:
```
go test ./...
```

## Deployment

1. Build the Docker image:
   ```
   docker build -t pubsub-amqp .
   ```

2. Run the container:
   ```
   docker run -p 8080:8080 --env-file .env pubsub-amqp
   ```

## Monitoring and Logging

- Prometheus metrics are exposed on the `/metrics` endpoint
- Jaeger UI can be used to view distributed traces
- Application logs are output to stdout/stderr

## Troubleshooting

- Check the application logs for error messages
- Ensure all required environment variables are set correctly
- Verify connectivity to external services (AMQP, S3, Redshift, Neo4j)

## Contributing

1. Fork the repository
2. Create a new branch for your feature
3. Commit your changes
4. Push to your fork
5. Create a pull request

## License

[MIT License](LICENSE)
