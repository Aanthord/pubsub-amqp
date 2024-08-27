package storage

import (
    "context"
    "database/sql"
    "fmt"

    "github.com/aanthord/pubsub-amqp/internal/tracing"
    _ "github.com/lib/pq"
    "go.uber.org/zap"
)

type RedshiftService interface {
    ExecuteQuery(ctx context.Context, query string) error
}

type redshiftService struct {
    db     *sql.DB
    logger *zap.SugaredLogger
}

func NewRedshiftService(connStr string, logger *zap.SugaredLogger) (RedshiftService, error) {
    db, err := sql.Open("postgres", connStr)
    if err != nil {
        return nil, fmt.Errorf("failed to connect to Redshift: %w", err)
    }

    return &redshiftService{
        db:     db,
        logger: logger,
    }, nil
}

func (s *redshiftService) ExecuteQuery(ctx context.Context, query string) error {
    span, ctx := tracing.StartSpanFromContext(ctx, "RedshiftQuery", "")
    defer span.Finish()

    _, err := s.db.ExecContext(ctx, query)
    if err != nil {
        span.SetTag("error", true)
        span.LogKV("event", "redshift_query_error", "error.message", err.Error())
        return fmt.Errorf("failed to execute Redshift query: %w", err)
    }

    span.LogKV("event", "redshift_query_success")
    return nil
}
