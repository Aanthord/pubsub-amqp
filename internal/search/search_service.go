package search

import (
    "context"
    "fmt"

    "github.com/aanthord/pubsub-amqp/internal/models"
    "github.com/aanthord/pubsub-amqp/internal/storage"
    "github.com/aanthord/pubsub-amqp/internal/tracing"
    "github.com/neo4j/neo4j-go-driver/v4/neo4j"
    "go.uber.org/zap"
)

type SearchService interface {
    SearchByTraceID(ctx context.Context, traceID string) ([]*models.MessagePayload, error)
}

type searchService struct {
    neo4jService storage.Neo4jService
    fileStorage  *storage.FileStorage
    logger       *zap.SugaredLogger
}

func NewSearchService(neo4jService storage.Neo4jService, fileStorage *storage.FileStorage, logger *zap.SugaredLogger) SearchService {
    return &searchService{
        neo4jService: neo4jService,
        fileStorage:  fileStorage,
        logger:       logger,
    }
}

func (s *searchService) SearchByTraceID(ctx context.Context, traceID string) ([]*models.MessagePayload, error) {
    span, ctx := tracing.StartSpanFromContext(ctx, "SearchByTraceID", traceID)
    defer span.Finish()

    // Search in Neo4j
    cypher := `
        MATCH (m:Message {trace_id: $traceID})
        RETURN m
        ORDER BY m.timestamp
    `
    params := map[string]interface{}{
        "traceID": traceID,
    }

    result, err := s.neo4jService.ExecuteQuery(ctx, cypher, params)
    if err != nil {
        span.SetTag("error", true)
        span.LogKV("event", "neo4j_search_error", "error.message", err.Error())
        return nil, fmt.Errorf("failed to execute Neo4j query: %w", err)
    }

    var messages []*models.MessagePayload
    for result.Next() {
        record := result.Record()
        messageNode := record.GetByIndex(0).(neo4j.Node)
        message := &models.MessagePayload{
            ID:        messageNode.Props["id"].(string),
            TraceID:   messageNode.Props["trace_id"].(string),
            Sender:    messageNode.Props["sender"].(string),
            Timestamp: messageNode.Props["timestamp"].(string),
            Version:   messageNode.Props["version"].(string),
            S3URI:     messageNode.Props["s3_uri"].(string),
            Retries:   int(messageNode.Props["retries"].(int64)),
        }
        messages = append(messages, message)
    }

    // Search in FileStorage
    fileMessages, err := s.fileStorage.Retrieve(ctx, traceID)
    if err != nil {
        span.SetTag("error", true)
        span.LogKV("event", "file_storage_search_error", "error.message", err.Error())
        return nil, fmt.Errorf("failed to search in file storage: %w", err)
    }

    messages = append(messages, fileMessages...)

    span.LogKV("event", "search_completed", "results_count", len(messages))
    return messages, nil
}
