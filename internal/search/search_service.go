package search

import (
	"context"
	"fmt"
	"time"

	"github.com/aanthord/pubsub-amqp/internal/metrics"
	"github.com/aanthord/pubsub-amqp/internal/models"
	"github.com/aanthord/pubsub-amqp/internal/storage"
	"go.uber.org/zap"
)

type SearchService interface {
	Search(ctx context.Context, query string) ([]models.SearchResult, error)
}

type searchService struct {
	neo4jService storage.Neo4jService
	logger       *zap.SugaredLogger
}

func NewSearchService(neo4jService storage.Neo4jService, logger *zap.SugaredLogger) SearchService {
	return &searchService{
		neo4jService: neo4jService,
		logger:       logger,
	}
}

func (s *searchService) Search(ctx context.Context, query string) ([]models.SearchResult, error) {
	start := time.Now()
	defer func() {
		metrics.SearchDuration.Observe(time.Since(start).Seconds())
	}()

	cypher := `
		MATCH (n)
		WHERE n.content CONTAINS $query
		RETURN n.id AS id, labels(n)[0] AS type, n.content AS content,
		       apoc.text.score(n.content, $query) AS score
		ORDER BY score DESC
		LIMIT 10
	`
	params := map[string]interface{}{
		"query": query,
	}

	result, err := s.neo4jService.ExecuteQuery(ctx, cypher, params)
	if err != nil {
		return nil, fmt.Errorf("failed to execute Neo4j query: %w", err)
	}

	var searchResults []models.SearchResult
	for result.Next() {
		record := result.Record()
		searchResult := models.SearchResult{
			ID:      record.GetByIndex(0).(string),
			Type:    record.GetByIndex(1).(string),
			Content: record.GetByIndex(2).(map[string]interface{}),
			Score:   record.GetByIndex(3).(float64),
		}
		searchResults = append(searchResults, searchResult)
	}

	s.logger.Infow("Search completed",
		"query", query,
		"resultsCount", len(searchResults),
	)

	metrics.SearchesTotal.Inc()
	return searchResults, nil
}
