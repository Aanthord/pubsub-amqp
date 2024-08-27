package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/aanthord/pubsub-amqp/internal/amqp"
	"github.com/aanthord/pubsub-amqp/internal/search"
	"github.com/aanthord/pubsub-amqp/internal/storage"
	"github.com/aanthord/pubsub-amqp/internal/tracing"
	"github.com/aanthord/pubsub-amqp/internal/uuid"
	"github.com/opentracing/opentracing-go"
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
}

func NewConfig() (*Config, error) {
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

	return &Config{
		AMQPService:     amqpService,
		S3Service:       s3Service,
		RedshiftService: redshiftService,
		Neo4jService:    neo4jService,
		UUIDService:     uuidService,
		SearchService:   searchService,
		Tracer:          tracer,
		TracerCloser:    closer,
	}, nil
}

// ... (rest of the file remains the same)
