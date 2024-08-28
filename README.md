# PubSub AMQP Service

## Overview

This project implements a robust Publish-Subscribe (PubSub) service using AMQP (Advanced Message Queuing Protocol). It's designed to handle high-throughput message processing with features like distributed tracing, metrics collection, and data persistence across various storage systems, with a focus on data integrity and traceability.

## Table of Contents

1. [Features](#features)
2. [Architecture](#architecture)
3. [Prerequisites](#prerequisites)
4. [Installation](#installation)
5. [Configuration](#configuration)
6. [Advanced Tracing Configuration](#advanced-tracing-configuration)
7. [AWS S3 Append-Only Configuration](#aws-s3-append-only-configuration)
8. [Usage](#usage)
9. [API Documentation](#api-documentation)
10. [Development](#development)
11. [Testing](#testing)
12. [Deployment](#deployment)
13. [Monitoring and Logging](#monitoring-and-logging)
14. [Troubleshooting](#troubleshooting)
15. [Contributing](#contributing)
16. [License](#license)

## Features

1. **AMQP-based Publish-Subscribe Messaging**: Robust message queuing using the Advanced Message Queuing Protocol.

2. **RESTful API**: For publishing and subscribing to topics, with full CRUD operations.

3. **Bidirectional Tracing**: 
   - Track the lifecycle of data both forwards and backwards, from creation to its final state.
   - Enables comprehensive auditing and data lineage tracking.

4. **Granular Traceability**: 
   - Capture detailed logs and traces for every function and sub-function within the system.
   - Categorized by topic and subtopic for easy navigation and analysis.

5. **Human-Readable Traces**: 
   - Each trace includes logical descriptions and human-readable data.
   - Ensures explainability and ease of understanding for both technical and non-technical users.

6. **Advanced Distributed Tracing**: 
   - Utilizes Jaeger for end-to-end distributed tracing.
   - Provides insights into system performance and bottlenecks.

7. **Comprehensive Metrics Collection**: 
   - Integrated with Prometheus for detailed metrics gathering and monitoring.

8. **Multi-tiered Data Persistence**:
   - Amazon S3: For large message payloads and file storage, with append-only and versioning capabilities.
   - Amazon Redshift: For analytics and historical data analysis.
   - Neo4j: For graph-based data relationships and Merkle tree structure imports.
   - Local File Storage: Utilizes 256 sharded flat files for efficient data storage and retrieval.

9. **UUID Management**: 
   - Generate and manage unique identifiers (UUIDs) for digital threads and messages.
   - Comprehensive search capabilities based on UUIDs.

10. **Metadata Management**: 
    - Store metadata with BLAKE3 hashes for files stored in S3.
    - Ensures secure and verifiable data integrity.

11. **Scalable and Optimized Storage**: 
    - 256 sharded flat files for efficient data storage and retrieval.
    - Optimized with Bloom filters for fast membership testing.
    - Utilizes memoization techniques to improve performance of repeated operations.

12. **Advanced Data Analysis and Visualization**:
    - Integration with AWS Redshift for complex querying and historical analysis.
    - Import of Merkle tree structures into Neo4j for graph-based visualization and analysis.

13. **Search Functionality**: 
    - Powerful search capabilities across all persisted data.
    - Utilizes indexing for fast retrieval.

14. **Configurable CORS Settings**: 
    - Flexible Cross-Origin Resource Sharing (CORS) configuration.
    - Ensures secure API access from various client-side applications.

15. **Comprehensive API Documentation**: 
    - Fully documented with Swagger/OpenAPI.
    - Interactive API exploration and testing interface.

16. **Dockerized Deployment**: 
    - Containerized application for easy deployment and scaling.
    - Includes all necessary components and dependencies.

17. **Secure Communication**: 
    - Supports HTTPS/TLS for encrypted data transmission.

18. **Flexible Configuration**: 
    - Extensive use of environment variables for easy configuration in different environments.

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
- Amazon S3 for large message storage with append-only and versioning capabilities
- Amazon Redshift for data warehousing
- Neo4j for graph database storage
- Jaeger for distributed tracing
- Prometheus for metrics collection
- Kafka for backing Jaeger spans (optional)

## Prerequisites

- Go 1.20 or later
- Docker and Docker Compose
- Access to AMQP server (e.g., RabbitMQ)
- AWS account with S3 and Redshift access
- Neo4j database
- Jaeger server for distributed tracing
- Prometheus server for metrics collection
- Kafka cluster (optional, for advanced tracing setup)
- `chattr` utility (for local append-only storage)

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

For advanced tracing configuration, including setting up Jaeger with Kafka as a backing store and using persistent storage with enhanced data integrity measures, see the [Advanced Tracing Configuration](#advanced-tracing-configuration) section.

## Advanced Tracing Configuration

### Setting up Jaeger with Kafka and Persistent Storage

For enhanced tracing capabilities and data persistence, you can set up Jaeger with Kafka as a backing store and use persistent storage with additional data integrity measures.

1. **Prerequisites**:
   - Kafka cluster
   - Persistent storage volume (e.g., EBS volume on AWS)
   - `chattr` utility (usually part of the `e2fsprogs` package)

2. **Kafka Setup**:
   - Ensure your Kafka cluster is running and accessible.
   - Create a topic for Jaeger spans:
     ```
     kafka-topics.sh --create --topic jaeger-spans --bootstrap-server localhost:9092 --partitions 5 --replication-factor 3
     ```

3. **Persistent Storage**:
   - Mount your persistent volume to a directory, e.g., `/data/jaeger`
   - Apply the append-only attribute to the data directory:
     ```
     sudo chattr +a /data/jaeger
     ```
   - This prevents accidental deletion and ensures data is only appended, not modified or removed.

4. **Jaeger Configuration**:
   - Update your Jaeger configuration to use Kafka and the persistent storage:

     ```yaml
     storage:
       type: kafka
       options:
         kafka:
           brokers: [kafka-broker1:9092, kafka-broker2:9092, kafka-broker3:9092]
           topic: jaeger-spans
           encoding: protobuf
     spanstore:
       type: badger
       options:
         directory: /data/jaeger
         value-directory: /data/jaeger
     ```

5. **Environment Variables**:
   Add the following to your `.env` file:
   ```
   JAEGER_STORAGE_TYPE=kafka
   JAEGER_KAFKA_BROKERS=kafka-broker1:9092,kafka-broker2:9092,kafka-broker3:9092
   JAEGER_KAFKA_TOPIC=jaeger-spans
   JAEGER_BADGER_DIRECTORY=/data/jaeger
   JAEGER_BADGER_VALUE_DIRECTORY=/data/jaeger
   ```

6. **Docker Compose Update**:
   If you're using Docker Compose, update your `docker-compose.yml` to include Jaeger with Kafka storage:

   ```yaml
   jaeger:
     image: jaegertracing/all-in-one:latest
     ports:
       - "16686:16686"
       - "14268:14268"
     environment:
       - SPAN_STORAGE_TYPE=kafka
       - KAFKA_BROKERS=kafka-broker1:9092,kafka-broker2:9092,kafka-broker3:9092
       - KAFKA_TOPIC=jaeger-spans
       - BADGER_DIRECTORY=/data/jaeger
       - BADGER_VALUE_DIRECTORY=/data/jaeger
     volumes:
       - /data/jaeger:/data/jaeger
     command: ["--badger.directory-value=/data/jaeger"]

   kafka:
     image: confluentinc/cp-kafka:latest
     # ... (Kafka configuration)

   zookeeper:
     image: confluentinc/cp-zookeeper:latest
     # ... (ZooKeeper configuration)
   ```

7. **Security Considerations**:
   - Ensure proper access controls on the `/data/jaeger` directory.
   - Use encryption at rest for the persistent volume.
   - Configure Kafka security (SSL/SASL) as per your security requirements.

8. **Monitoring and Maintenance**:
   - Regularly monitor the size of your persistent storage and Kafka topic.
   - Implement a retention policy for your Kafka topic to manage data growth.
   - Periodically check the integrity of the append-only attribute:
     ```
     lsattr /data/jaeger
     ```

## AWS S3 Append-Only Configuration

To ensure data integrity and maintain a complete history of all operations, we configure the S3 bucket used for storage to be append-only with versioning enabled. This prevents data deletion and modifications to existing objects.

### S3 Bucket Policy

Apply the following bucket policy to your S3 bucket:

```json
{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Sid": "AppendOnlyAccess",
            "Effect": "Allow",
            "Principal": {
                "AWS": "arn:aws:iam::YOUR_ACCOUNT_ID:user/YOUR_IAM_USER"
            },
            "Action": [
                "s3:PutObject",
                "s3:GetBucketLocation",
                "s3:ListBucket",
                "s3:GetObject",
                "s3:GetObjectVersion"
            ],
            "Resource": [
                "arn:aws:s3:::YOUR_BUCKET_NAME",
                "arn:aws:s3:::YOUR_BUCKET_NAME/*"
            ]
        },
        {
            "Sid": "DenyObjectDeletion",
            "Effect": "Deny",
            "Principal": "*",
            "Action": [
                "s3:DeleteObject",
                "s3:DeleteObjectVersion"
            ],
            "Resource": "arn:aws:s3:::YOUR_BUCKET_NAME/*"
        }
    ]
}
```

### Instructions

1. Create an S3 bucket in your AWS account.
2. Enable versioning for the bucket:
   - Go to the S3 console, select your bucket, and navigate to the "Properties" tab.
   - Under "Bucket Versioning", click "Edit" and enable versioning.
3. Apply the bucket policy:
   - In the S3 console, select your bucket and go to the "Permissions" tab.
   - Under "Bucket policy", click "Edit" and paste the policy above.
   - Replace `YOUR_ACCOUNT_ID`, `YOUR_IAM_USER`, and `YOUR_BUCKET_NAME` with your actual values.
4. Update your application's AWS credentials to use an IAM user that has the permissions specified in the bucket policy.

This configuration ensures that:
- Only authorized users can write new objects and read existing ones.
- No one can delete objects or their versions.
- All modifications result in new versions, preserving the entire history.

## Usage

1. Start the application:
   ```
   ./app
   ```

2. The server will start on the configured port (default: 8080).

3. Use the provided API endpoints to publish and subscribe to topics.

## API Documentation

Swagger documentation is available at `/swagger/index.html`
