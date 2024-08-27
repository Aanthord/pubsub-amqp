package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/aanthord/pubsub-amqp/internal/amqp"
	"github.com/aanthord/pubsub-amqp/internal/search"
	"github.com/aanthord/pubsub-amqp/internal/storage"
	"github.com/aanthord/pubsub-amqp/internal/uuid"
	"github.com/opentracing/opentracing-go"
	"go.uber.org/zap"
)

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
	S3OffloadLimit  int64 // In bytes, -1 means always offload to S3
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

	uuidService := uuid.NewUUIDService()

	searchService := search.NewSearchService(neo4jService)

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

func GetEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

func GetEnvAsInt(key string, defaultValue int) int {
	valueStr := GetEnv(key, "")
	if value, err := strconv.Atoi(valueStr); err == nil {
		return value
	}
	return defaultValue
}

func GetEnvAsInt64(key string, defaultValue int64) int64 {
	valueStr := GetEnv(key, "")
	if value, err := strconv.ParseInt(valueStr, 10, 64); err == nil {
		return value
	}
	return defaultValue
}

func GetEnvAsBool(key string, defaultValue bool) bool {
	valueStr := GetEnv(key, "")
	if value, err := strconv.ParseBool(valueStr); err == nil {
		return value
	}
	return defaultValue
}
