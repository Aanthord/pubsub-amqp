package config

import (
    "fmt"
    "io"
    "os"
    "strconv"

    "github.com/aanthord/pubsub-amqp/internal/types"
    "github.com/opentracing/opentracing-go"
    "go.uber.org/zap"
)

type Config struct {
    AMQPService     types.AMQPService
    S3Service       types.S3Service
    RedshiftService types.RedshiftService
    Neo4jService    types.Neo4jService
    UUIDService     types.UUIDService
    SearchService   types.SearchService
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

    amqpService, err := NewAMQPService()
    if err != nil {
        return nil, fmt.Errorf("failed to initialize AMQP service: %w", err)
    }

    s3Service, err := NewS3Service()
    if err != nil {
        return nil, fmt.Errorf("failed to initialize S3 service: %w", err)
    }

    redshiftService, err := NewRedshiftService()
    if err != nil {
        return nil, fmt.Errorf("failed to initialize Redshift service: %w", err)
    }

    neo4jService, err := NewNeo4jService()
    if err != nil {
        return nil, fmt.Errorf("failed to initialize Neo4j service: %w", err)
    }

    uuidService := NewUUIDService()

    searchService := NewSearchService(neo4jService)

    tracer, closer, err := InitJaeger("pubsub-amqp")
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

func GetEnv(key, defaultValue string) string {
    if value, exists := os.LookupEnv(key); exists {
        return value
    }
    return defaultValue
}

func getEnvAsInt64(key string, defaultValue int64) int64 {
    valueStr := GetEnv(key, "")
    if value, err := strconv.ParseInt(valueStr, 10, 64); err == nil {
        return value
    }
    return defaultValue
}
