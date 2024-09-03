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
    neo4jService    storage.Neo4jService
    fileStorage     *storage.FileStorage
    s3Service       storage.S3Service
    redshiftService storage.RedshiftService
    logger          *zap.SugaredLogger
}

func NewSearchService(
    neo4jService storage.Neo4jService,
    fileStorage *storage.FileStorage,
    s3Service storage.S3Service,
    redshiftService storage.RedshiftService,
    logger *zap.SugaredLogger,
) SearchService {
    return &searchService{
        neo4jService:    neo4jService,
        fileStorage:     fileStorage,
        s3Service:       s3Service,
        redshiftService: redshiftService,
        logger:          logger,
    }
}

func (s *searchService) SearchByTraceID(ctx context.Context, traceID string) ([]*models.MessagePayload, error) {
    span, ctx := tracing.StartSpanFromContext(ctx, "SearchByTraceID", traceID)
    defer span.Finish()

    var messages []*models.MessagePayload

    // Search in Neo4j
    neoMessages, err := s.searchNeo4j(ctx, traceID)
    if err != nil {
        s.logger.Errorw("Failed to search in Neo4j", "error", err)
        // Continue with other searches even if Neo4j search fails
    }
    messages = append(messages, neoMessages...)

    // Search in FileStorage
    fileMessages, err := s.fileStorage.Retrieve(ctx, traceID)
    if err != nil {
        s.logger.Errorw("Failed to search in file storage", "error", err)
        // Continue with other searches even if file storage search fails
    }
    messages = append(messages, fileMessages...)

    // Search in S3 if service is available
    if s.s3Service != nil {
        s3Messages, err := s.searchS3(ctx, traceID)
        if err != nil {
            s.logger.Errorw("Failed to search in S3", "error", err)
            // Continue with other searches even if S3 search fails
        }
        messages = append(messages, s3Messages...)
    }

    // Search in Redshift if service is available
    if s.redshiftService != nil {
        redshiftMessages, err := s.searchRedshift(ctx, traceID)
        if err != nil {
            s.logger.Errorw("Failed to search in Redshift", "error", err)
            // Continue with other searches even if Redshift search fails
        }
        messages = append(messages, redshiftMessages...)
    }

    span.LogKV("event", "search_completed", "results_count", len(messages))
    return messages, nil
}

func (s *searchService) searchNeo4j(ctx context.Context, traceID string) ([]*models.MessagePayload, error) {
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
        return nil, fmt.Errorf("failed to execute Neo4j query: %w", err)
    }

    var messages []*models.MessagePayload
    for result.Next() {
        record := result.Record()
        messageNode := record.GetByIndex(0).(neo4j.Node)
        message := &models.MessagePayload{
            Header: models.HeaderData{
                ID:        messageNode.Props["id"].(string),
                TraceID:   messageNode.Props["trace_id"].(string),
                Sender:    messageNode.Props["sender"].(string),
                Timestamp: messageNode.Props["timestamp"].(string),
                Version:   messageNode.Props["version"].(string),
                Retries:   int(messageNode.Props["retries"].(int64)),
            },
            Content: models.Content{
                Data: map[string]interface{}{"s3_uri": messageNode.Props["s3_uri"].(string)},
            },
        }
        messages = append(messages, message)
    }

    return messages, nil
}

func (s *searchService) searchS3(ctx context.Context, traceID string) ([]*models.MessagePayload, error) {
    // Implement S3 search logic here
    // This is a placeholder implementation
    return nil, nil
}

func (s *searchService) searchRedshift(ctx context.Context, traceID string) ([]*models.MessagePayload, error) {
    // Implement Redshift search logic here
    // This is a placeholder implementation
    return nil, nil
}
