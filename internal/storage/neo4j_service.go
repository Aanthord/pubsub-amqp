package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/aanthord/pubsub-amqp/internal/metrics"
	"github.com/aanthord/pubsub-amqp/internal/types"
	"github.com/neo4j/neo4j-go-driver/v4/neo4j"
	"go.uber.org/zap"
)

type Neo4jService interface {
	ExecuteQuery(ctx context.Context, query string, params map[string]interface{}) (neo4j.Result, error)
}

type neo4jService struct {
	driver neo4j.Driver
	logger *zap.SugaredLogger
}

func NewNeo4jService(config types.ConfigProvider) (Neo4jService, error) {
	logger, _ := zap.NewProduction()
	sugar := logger.Sugar()

	driver, err := neo4j.NewDriver(
		config.GetNeo4jURI(),
		neo4j.BasicAuth(
			config.GetNeo4jUsername(),
			config.GetNeo4jPassword(),
			"",
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Neo4j: %w", err)
	}

	return &neo4jService{driver: driver, logger: sugar}, nil
}

func (s *neo4jService) ExecuteQuery(ctx context.Context, query string, params map[string]interface{}) (neo4j.Result, error) {
	start := time.Now()
	defer func() {
		metrics.Neo4jQueryDuration.Observe(time.Since(start).Seconds())
	}()

	session := s.driver.NewSession(neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close()

	result, err := session.RunWithContext(ctx, query, params)
	if err != nil {
		s.logger.Errorw("Failed to execute Neo4j query", "error", err)
		return nil, fmt.Errorf("failed to execute Neo4j query: %w", err)
	}

	s.logger.Infow("Neo4j query executed successfully")
	metrics.Neo4jQueriesTotal.Inc()
	return result, nil
}
