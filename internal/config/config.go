package config

import (
    "fmt"
    "io"
    "os"
    "strconv"

    "github.com/aanthord/pubsub-amqp/internal/amqp"
    "github.com/aanthord/pubsub-amqp/internal/storage"
    "github.com/aanthord/pubsub-amqp/internal/tracing"
    "github.com/opentracing/opentracing-go"
    "go.uber.org/zap"
)

type Config struct {
    AMQPService     amqp.AMQPService
    S3Service       storage.S3Service
    RedshiftService storage.RedshiftService
    Neo4jService    storage.Neo4jService
    UUIDService     storage.UUIDService
    SearchService   storage.SearchService
    Tracer          opentracing.Tracer
    TracerCloser    io.Closer
    Logger          *zap.SugaredLogger
    S3OffloadLimit  int64
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

    s3Service, err := storage.NewS3Service()
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
