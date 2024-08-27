package search

import (
	"context"
	"fmt"
	"time"

	"github.com/aanthord/pubsub-amqp/internal/metrics"
	"github.com/aanthord/pubsub-amqp/internal/storage"
	"github.com/neo4j/neo4j-go-driver/v4/neo4j"
	"go.uber.org/zap"
)

type SearchService interface {
	Search(ctx context.Context, query string) ([]neo4j.Record, error)
}

type searchService struct {
	neo4jService storage.Neo4jService
	logger       *zap.SugaredLogger
}

func NewSearchService(neo4jService storage.Neo4jService) SearchService {
	logger, _ := zap.NewProduction()
	sugar := logger.Sugar()

	return &searchService{
		neo4jService: neo4jService,
		logger:       sugar,
	}
}

func (s *searchService) Search(ctx context.Context, query string) ([]neo4j.Record, error) {
	start := time.Now()
	defer func() {
		metrics.SearchDuration.Observe(time.Since(start).Seconds())
	}()

	cypher := "MATCH (n) WHERE n.name CONTAINS $query RETURN n"
	params := map[string]interface{}{
		"query": query,
	}

	result, err := s.neo4jService.ExecuteQuery(ctx, cypher, params)
	if err != nil {
		s.logger.Errorw("Failed to execute search query", "error", err, "query", query)
		return nil, fmt.Errorf("failed to execute search query: %w", err)
	}

	records, err := result.Collect()
	if err != nil {
		s.logger.Errorw("Failed to collect search results", "error", err)
		return nil, fmt.Errorf("failed to collect search results: %w", err)
	}

	s.logger.Infow("Search executed successfully", "query", query, "results", len(records))
	metrics.SearchesTotal.Inc()
	return records, nil
}
