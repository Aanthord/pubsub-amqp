package config

import (
    "fmt"
    "io"
    "log"
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
    ShardDepth         int
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

func (c *AppConfig) GetShardDepth() int {
    return c.ShardDepth
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
    EnableS3        bool
    EnableRedshift  bool
}

func NewConfig() (*Config, error) {
    fmt.Println("Starting configuration initialization...")

    logger, err := zap.NewProduction()
    if err != nil {
        fmt.Printf("Failed to initialize logger: %v\n", err)
        return nil, fmt.Errorf("failed to initialize logger: %w", err)
    }
    sugar := logger.Sugar()
    sugar.Info("Logger initialized")
    fmt.Println("Logger initialized")

    shardDepth, err := strconv.Atoi(mustGetEnv("SHARD_DEPTH"))
    if err != nil {
        log.Fatalf("Invalid shard depth value: %v", err)
    }

    appConfig := &AppConfig{
        AWSRegion:          mustGetEnv("AWS_REGION"),
        S3Bucket:           mustGetEnv("AWS_S3_BUCKET"),
        Neo4jURI:           mustGetEnv("NEO4J_URI"),
        Neo4jUsername:      mustGetEnv("NEO4J_USERNAME"),
        Neo4jPassword:      mustGetEnv("NEO4J_PASSWORD"),
        RedshiftConnString: mustGetEnv("REDSHIFT_CONN_STRING"),
        FileStoragePath:    mustGetEnv("FILE_STORAGE_PATH"),
        ShardDepth:         shardDepth,
    }
    sugar.Infow("AppConfig initialized", "config", appConfig)
    fmt.Printf("AppConfig initialized: %+v\n", appConfig)

    amqpURL := mustGetEnv("AMQP_URL")
    amqpService, err := initializeService(func() (amqp.AMQPService, error) {
        return amqp.NewAMQPService(amqpURL, sugar)
    }, sugar, "AMQP")
    if err != nil {
        return nil, err
    }

    enableS3 := getEnvAsBool("ENABLE_S3", false)
    enableRedshift := getEnvAsBool("ENABLE_REDSHIFT", false)

    cfg := &Config{
        AMQPService:    amqpService,
        EnableS3:       enableS3,
        EnableRedshift: enableRedshift,
        Logger:         sugar,
    }

    if enableS3 {
        s3Service, err := initializeService(func() (storage.S3Service, error) {
            return storage.NewS3Service(appConfig)
        }, sugar, "S3")
        if err != nil {
            return nil, err
        }
        cfg.S3Service = s3Service
    } else {
        sugar.Info("S3 service disabled")
        fmt.Println("S3 service disabled")
    }

    if enableRedshift {
        redshiftService, err := initializeService(func() (storage.RedshiftService, error) {
            return storage.NewRedshiftService(appConfig)
        }, sugar, "Redshift")
        if err != nil {
            return nil, err
        }
        cfg.RedshiftService = redshiftService
    } else {
        sugar.Info("Redshift service disabled")
        fmt.Println("Redshift service disabled")
    }

    neo4jService, err := initializeService(func() (storage.Neo4jService, error) {
        return storage.NewNeo4jService(appConfig)
    }, sugar, "Neo4j")
    if err != nil {
        return nil, err
    }
    cfg.Neo4jService = neo4jService

    cfg.UUIDService = uuid.NewUUIDService()
    sugar.Info("UUID service initialized")
    fmt.Println("UUID service initialized")

    fileStorage, err := initializeService(func() (*storage.FileStorage, error) {
        return storage.NewFileStorage(appConfig.GetFileStoragePath(), sugar, appConfig.GetShardDepth())
    }, sugar, "File Storage")
    if err != nil {
        return nil, err
    }

    cfg.SearchService = search.NewSearchService(
        neo4jService,
        fileStorage,
        cfg.S3Service,
        cfg.RedshiftService,
        sugar,
    )
    sugar.Info("Search service initialized")
    fmt.Println("Search service initialized")

    tracer, closer, err := tracing.InitJaeger("pubsub-amqp")
    if err != nil {
        sugar.Errorw("Failed to initialize Jaeger tracer", "error", err)
        fmt.Printf("Failed to initialize Jaeger tracer: %v\n", err)
        return nil, fmt.Errorf("failed to initialize Jaeger tracer: %w", err)
    }
    cfg.Tracer = tracer
    cfg.TracerCloser = closer
    sugar.Info("Jaeger tracer initialized")
    fmt.Println("Jaeger tracer initialized")

    cfg.S3OffloadLimit = getEnvAsInt64("S3_OFFLOAD_LIMIT", -1)
    sugar.Infow("S3 offload limit set", "limit", cfg.S3OffloadLimit)
    fmt.Printf("S3 offload limit set: %d\n", cfg.S3OffloadLimit)

    sugar.Info("Configuration initialization completed")
    fmt.Println("Configuration initialization completed")

    return cfg, nil
}

// initializeService is a helper function to initialize services with logging and error handling
func initializeService[T any](serviceInitFunc func() (T, error), logger *zap.SugaredLogger, serviceName string) (T, error) {
    service, err := serviceInitFunc()
    if err != nil {
        logger.Errorw(fmt.Sprintf("Failed to initialize %s service", serviceName), "error", err)
        fmt.Printf("Failed to initialize %s service: %v\n", serviceName, err)
        return service, fmt.Errorf("failed to initialize %s service: %w", serviceName, err)
    }
    logger.Info(fmt.Sprintf("%s service initialized", serviceName))
    fmt.Println(fmt.Sprintf("%s service initialized", serviceName))
    return service, nil
}

// mustGetEnv retrieves an environment variable or logs a fatal error if it's not set
func mustGetEnv(key string) string {
    value := os.Getenv(key)
    if value == "" {
        log.Fatalf("Environment variable %s is not set", key)
    }
    return value
}

func getEnv(key, defaultValue string) string {
    value := os.Getenv(key)
    if value == "" {
        log.Printf("Environment variable %s not set, using default: %s", key, defaultValue)
        return defaultValue
    }
    return value
}

func getEnvAsBool(key string, defaultValue bool) bool {
    valStr := getEnv(key, "")
    if val, err := strconv.ParseBool(valStr); err == nil {
        return val
    }
    return defaultValue
}

func getEnvAsInt64(key string, defaultValue int64) int64 {
    valueStr := os.Getenv(key)
    if valueStr == "" {
        log.Printf("Environment variable %s not set, using default: %d", key, defaultValue)
        return defaultValue
    }
    value, err := strconv.ParseInt(valueStr, 10, 64)
    if err != nil {
        log.Printf("Invalid value for environment variable %s: %v. Using default: %d", key, err, defaultValue)
        return defaultValue
    }
    return value
}
