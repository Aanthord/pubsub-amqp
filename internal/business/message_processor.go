package business

import (
    "context"
    "encoding/json"
    "fmt"
    "time"

    "github.com/aanthord/pubsub-amqp/internal/amqp"
    "github.com/aanthord/pubsub-amqp/internal/models"
    "github.com/aanthord/pubsub-amqp/internal/storage"
    "github.com/aanthord/pubsub-amqp/internal/tracing"
    "github.com/cenkalti/backoff/v4"
    "go.uber.org/zap"
)

type MessageProcessor struct {
    amqpService     amqp.AMQPService
    s3Service       storage.S3Service
    redshiftService storage.RedshiftService
    fileStorage     *storage.FileStorage
    logger          *zap.SugaredLogger
    s3OffloadLimit  int64
}

func NewMessageProcessor(amqpService amqp.AMQPService, s3Service storage.S3Service, redshiftService storage.RedshiftService, fileStorage *storage.FileStorage, logger *zap.SugaredLogger, s3OffloadLimit int64) *MessageProcessor {
    return &MessageProcessor{
        amqpService:     amqpService,
        s3Service:       s3Service,
        redshiftService: redshiftService,
        fileStorage:     fileStorage,
        logger:          logger,
        s3OffloadLimit:  s3OffloadLimit,
    }
}

func (mp *MessageProcessor) ProcessMessage(ctx context.Context, topic string, message *models.MessagePayload) error {
    span, ctx := tracing.StartSpanFromContext(ctx, "ProcessMessage")
    defer span.Finish()

    jsonMessage, err := json.Marshal(message)
    if err != nil {
        return fmt.Errorf("failed to marshal message: %w", err)
    }

    if mp.s3OffloadLimit == -1 || int64(len(jsonMessage)) > mp.s3OffloadLimit {
        err = mp.offloadToS3(ctx, topic, message, jsonMessage)
        if err != nil {
            return err
        }
    }

    err = mp.storeMessage(ctx, message)
    if err != nil {
        return err
    }

    err = mp.publishToAMQP(ctx, topic, jsonMessage)
    if err != nil {
        return err
    }

    mp.logger.Infow("Message processed successfully",
        "id", message.ID,
        "sender", message.Sender,
        "timestamp", message.Timestamp,
        "topic", topic,
        "s3URI", message.S3URI,
        "traceID", message.TraceID,
    )

    return nil
}

func (mp *MessageProcessor) offloadToS3(ctx context.Context, topic string, message *models.MessagePayload, jsonMessage []byte) error {
    span, ctx := tracing.StartSpanFromContext(ctx, "OffloadToS3")
    defer span.Finish()

    s3Key := fmt.Sprintf("messages/%s/%s.json", topic, message.Timestamp)
    s3URI, err := mp.s3Service.UploadFile(ctx, s3Key, jsonMessage)
    if err != nil {
        return fmt.Errorf("failed to upload message to S3: %w", err)
    }
    
    message.S3URI = s3URI
    message.Content = nil
    
    return nil
}

func (mp *MessageProcessor) storeMessage(ctx context.Context, message *models.MessagePayload) error {
    span, ctx := tracing.StartSpanFromContext(ctx, "StoreMessage")
    defer span.Finish()

    err := backoff.Retry(func() error {
        return mp.fileStorage.Store(message)
    }, backoff.NewExponentialBackOff())

    if err != nil {
        return fmt.Errorf("failed to store message in file storage: %w", err)
    }

    query := fmt.Sprintf(`
        INSERT INTO messages (id, sender, timestamp, version, s3_uri, trace_id, retries)
        VALUES ('%s', '%s', '%s', '%s', '%s', '%s', %d)
    `, message.ID, message.Sender, message.Timestamp, message.Version, message.S3URI, message.TraceID, message.Retries)
    
    err = backoff.Retry(func() error {
        return mp.redshiftService.ExecuteQuery(ctx, query)
    }, backoff.NewExponentialBackOff())

    if err != nil {
        return fmt.Errorf("failed to insert message metadata into Redshift: %w", err)
    }

    return nil
}

func (mp *MessageProcessor) publishToAMQP(ctx context.Context, topic string, jsonMessage []byte) error {
    span, ctx := tracing.StartSpanFromContext(ctx, "PublishToAMQP")
    defer span.Finish()

    err := backoff.Retry(func() error {
        return mp.amqpService.PublishMessage(ctx, topic, jsonMessage)
    }, backoff.NewExponentialBackOff())

    if err != nil {
        return fmt.Errorf("failed to publish message to AMQP: %w", err)
    }

    return nil
}
