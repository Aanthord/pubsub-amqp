package storage

import (
    "database/sql"
    "fmt"
    "github.com/aanthord/pubsub-amqp/internal/metrics"
    "github.com/aanthord/pubsub-amqp/internal/types"
    _ "github.com/lib/pq"
    "time"
    "go.uber.org/zap"
)

type RedshiftService interface {
    ExecuteQuery(query string) error
}

type redshiftService struct {
    db     *sql.DB
    logger *zap.SugaredLogger
}

func NewRedshiftService(config types.ConfigProvider) (RedshiftService, error) {
    logger, _ := zap.NewProduction()
    sugar := logger.Sugar()

    connStr := config.GetRedshiftConnString()
    db, err := sql.Open("postgres", connStr)
    if err != nil {
        return nil, fmt.Errorf("failed to connect to Redshift: %w", err)
    }

    return &redshiftService{db: db, logger: sugar}, nil
}

func (r *redshiftService) ExecuteQuery(query string) error {
    start := time.Now()
    defer func() {
        metrics.RedshiftQueryDuration.Observe(time.Since(start).Seconds())
    }()

    _, err := r.db.Exec(query)
    if err != nil {
        r.logger.Errorw("Failed to execute Redshift query", "error", err)
        return fmt.Errorf("failed to execute Redshift query: %w", err)
    }

    r.logger.Infow("Redshift query executed successfully")
    metrics.RedshiftQueriesTotal.Inc()
    return nil
}
