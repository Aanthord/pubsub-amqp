package config

import (
    "fmt"
    "io"
    "os"
    "strconv"

    "github.com/aanthord/pubsub-amqp/internal/amqp"
    "github.com/aanthord/pubsub-amqp/internal/search"
    "github.com/aanthord/pubsub-amqp/internal/storage"
    "github.com/aanthord/pubsub-amqp/internal/tracing"
    "github.com/aanthord/pubsub-amqp/internal/uuid"
    "github.com/opentracing/opentracing-go"
    "go.uber.org/zap"
)

type AppConfig struct {
    AWSRegion          string
    S3Bucket           string
    Neo4jURI           string
    Neo4jUsername      string
    Neo4jPassword      string
    RedshiftConnString string
    FileStoragePath    string
}

func (c *AppConfig) GetAWSRegion() string {
    return c.AWSRegion
}

func (c *AppConfig) GetS3Bucket() string {
    return c.S3Bucket
}

func (c *AppConfig) GetNeo4jURI() string {
    return c.Neo4jURI
}

func (c *AppConfig) GetNeo4jUsername() string {
    return c.Neo4jUsername
}

func (c *AppConfig) GetNeo4jPassword() string {
    return c.Neo4jPassword
}

func (c *AppConfig) GetRedshiftConnString() string {
    return c.RedshiftConnString
}

func (c *AppConfig) GetFileStoragePath() string {
    return c.FileStoragePath
}

type Config struct {
    AMQPService     amqp.AMQPService
    S3Service       storage.S3Service
    RedshiftService storage.RedshiftService
    Neo4jService    storage.Neo4jService
    UUIDService     uuid.UUIDService
    SearchService   search.SearchService
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

    appConfig := &AppConfig{
        AWSRegion:          getEnv("AWS_REGION", "us-west-2"),
        S3Bucket:           getEnv("AWS_S3_BUCKET", "my-default-bucket"),
        Neo4jURI:           getEnv("NEO4J_URI", "bolt://localhost:7687"),
        Neo4jUsername:      getEnv("NEO4J_USERNAME", "neo4j"),
        Neo4jPassword:      getEnv("NEO4J_PASSWORD", ""),
        RedshiftConnString: getEnv("REDSHIFT_CONN_STRING", "user=username dbname=mydb sslmode=disable"),
        FileStoragePath:    getEnv("FILE_STORAGE_PATH", "/data/files"),
    }

    amqpURL := getEnv("AMQP_URL", "amqp://guest:guest@localhost:5672/")
    amqpService, err := amqp.NewAMQPService(amqpURL, sugar)
    if err != nil {
        return nil, fmt.Errorf("failed to initialize AMQP service: %w", err)
    }

    s3Service, err := storage.NewS3Service(appConfig)
    if err != nil {
        return nil, fmt.Errorf("failed to initialize S3 service: %w", err)
    }

    redshiftService, err := storage.NewRedshiftService(appConfig)
    if err != nil {
        return nil, fmt.Errorf("failed to initialize Redshift service: %w", err)
    }

    neo4jService, err := storage.NewNeo4jService(appConfig)
    if err != nil {
        return nil, fmt.Errorf("failed to initialize Neo4j service: %w", err)
    }

    uuidService := uuid.NewUUIDService()

    // Initialize FileStorage with the required arguments
    fileStorage, err := storage.NewFileStorage(appConfig.GetFileStoragePath(), sugar)
    if err != nil {
        return nil, fmt.Errorf("failed to initialize File Storage: %w", err)
    }

    // Pass the required arguments to NewSearchService
    searchService := search.NewSearchService(neo4jService, fileStorage, sugar)

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

func getEnv(key string, defaultValue string) string {
    if value := os.Getenv(key); value != "" {
        return value
    }
    return defaultValue
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
