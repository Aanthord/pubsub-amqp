package business

import (
    "context"
    "encoding/json"
    "fmt"

    "github.com/aanthord/pubsub-amqp/internal/amqp"
    "github.com/aanthord/pubsub-amqp/internal/models"
    "github.com/aanthord/pubsub-amqp/internal/storage"
    "github.com/aanthord/pubsub-amqp/internal/tracing"
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
    span, ctx := tracing.StartSpanFromContext(ctx, "ProcessMessage", message.TraceID)
    defer span.Finish()

    jsonMessage, err := json.Marshal(message)
    if err != nil {
        span.SetTag("error", true)
        span.LogKV("event", "marshal_error", "error.message", err.Error())
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

    err = mp.publishToAMQP(ctx, topic, message)
    if err != nil {
        return err
    }

    mp.logger.Infow("Message processed successfully",
        "id", message.ID,
        "trace_id", message.TraceID,
        "sender", message.Sender,
        "timestamp", message.Timestamp,
        "topic", topic,
        "s3URI", message.S3URI,
    )

    return nil
}

func (mp *MessageProcessor) offloadToS3(ctx context.Context, topic string, message *models.MessagePayload, jsonMessage []byte) error {
    span, ctx := tracing.StartSpanFromContext(ctx, "OffloadToS3", message.TraceID)
    defer span.Finish()

    s3Key := fmt.Sprintf("messages/%s/%s.json", topic, message.Timestamp)
    s3URI, err := mp.s3Service.UploadFile(ctx, s3Key, jsonMessage, message.TraceID)
    if err != nil {
        span.SetTag("error", true)
        span.LogKV("event", "s3_upload_error", "error.message", err.Error())
        return fmt.Errorf("failed to upload message to S3: %w", err)
    }
    
    message.S3URI = s3URI
    message.Content = nil
    
    span.LogKV("event", "s3_upload_success", "s3_uri", s3URI)
    return nil
}

func (mp *MessageProcessor) storeMessage(ctx context.Context, message *models.MessagePayload) error {
    span, ctx := tracing.StartSpanFromContext(ctx, "StoreMessage", message.TraceID)
    defer span.Finish()

    err := mp.fileStorage.Store(ctx, message)
    if err != nil {
        span.SetTag("error", true)
        span.LogKV("event", "file_storage_error", "error.message", err.Error())
        return fmt.Errorf("failed to store message in file storage: %w", err)
    }

    query := fmt.Sprintf(`
        INSERT INTO messages (id, trace_id, sender, timestamp, version, s3_uri, retries)
        VALUES ('%s', '%s', '%s', '%s', '%s', '%s', %d)
    `, message.ID, message.TraceID, message.Sender, message.Timestamp, message.Version, message.S3URI, message.Retries)
    
    // Call ExecuteQuery with the SQL query string only
    err = mp.redshiftService.ExecuteQuery(query)
    if err != nil {
        span.SetTag("error", true)
        span.LogKV("event", "redshift_error", "error.message", err.Error())
        return fmt.Errorf("failed to insert message metadata into Redshift: %w", err)
    }

    span.LogKV("event", "message_stored")
    return nil
}

func (mp *MessageProcessor) publishToAMQP(ctx context.Context, topic string, message *models.MessagePayload) error {
    span, ctx := tracing.StartSpanFromContext(ctx, "PublishToAMQP", message.TraceID)
    defer span.Finish()

    err := mp.amqpService.PublishMessage(ctx, topic, message)
    if err != nil {
        span.SetTag("error", true)
        span.LogKV("event", "amqp_publish_error", "error.message", err.Error())
        return fmt.Errorf("failed to publish message to AMQP: %w", err)
    }

    span.LogKV("event", "message_published")
    return nil
}
