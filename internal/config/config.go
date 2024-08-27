// config/config.go
package config

import (
    "fmt"
    "io"
    "os"
    "strconv"

    "github.com/aanthord/pubsub-amqp/internal/amqp"
    "github.com/aanthord/pubsub-amqp/internal/storage"
    "github.com/aanthord/pubsub-amqp/internal/tracing"
    "github.com/aanthord/pubsub-amqp/internal/types"
    "github.com/opentracing/opentracing-go"
    "go.uber.org/zap"
)

type AppConfig struct {
    AWSRegion string
    S3Bucket  string
}

func (c *AppConfig) GetAWSRegion() string {
    return c.AWSRegion
}

func (c *AppConfig) GetS3Bucket() string {
    return c.S3Bucket
}

func NewConfig() (*Config, error) {
    logger, err := zap.NewProduction()
    if err != nil {
        return nil, fmt.Errorf("failed to initialize logger: %w", err)
    }
    sugar := logger.Sugar()

    amqpService, err := amqp.NewAMQPService()
    if err != nil {
        return nil, fmt.Errorf("failed to initialize AMQP service: %w", err)
    }

    cfg := &AppConfig{
        AWSRegion: os.Getenv("AWS_REGION"),
        S3Bucket:  os.Getenv("AWS_S3_BUCKET"),
    }

    s3Service, err := storage.NewS3Service(cfg)
    if err != nil {
        return nil, fmt.Errorf("failed to initialize S3 service: %w", err)
    }

    redshiftService, err := storage.NewRedshiftService()
    if err != nil {
        return nil, fmt.Errorf("failed to initialize Redshift service: %w", err)
    }

    neo4jService, err := storage.NewNeo4jService()
    if err != nil {
        return nil, fmt.Errorf("failed to initialize Neo4j service: %w", err)
    }

    uuidService := storage.NewUUIDService()

    searchService := storage.NewSearchService()

    tracer, closer, err := tracing.InitJaeger("pubsub-amqp")
    if err != nil {
        return nil, fmt.Errorf("failed to initialize Jaeger tracer: %w", err)
    }

    s3OffloadLimit := getEnvAsInt64("S3_OFFLOAD_LIMIT", -1)

    return &Config{
        AMQPService:     amqpService,
        S3Service:       s3Service,
        RedshiftService: redshiftService,
        Neo4jService:    neo4jService,
        UUIDService:     uuidService,
        SearchService:   searchService,
        Tracer:          tracer,
        TracerCloser:    closer,
        Logger:          sugar,
        S3OffloadLimit:  s3OffloadLimit,
    }, nil
}

func getEnvAsInt64(key string, defaultValue int64) int64 {
    valueStr := os.Getenv(key)
    if valueStr == "" {
        return defaultValue
    }
    value, err := strconv.ParseInt(valueStr, 10, 64)
    if err != nil {
        return defaultValue
    }
    return value
}
