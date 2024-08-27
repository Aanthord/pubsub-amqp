package business

import (
    "context"
    "encoding/json"
    "fmt"

    "github.com/aanthord/pubsub-amqp/internal/amqp"
    "github.com/aanthord/pubsub-amqp/internal/models"
    "github.com/aanthord/pubsub-amqp/internal/storage"
    "go.uber.org/zap"
)

type MessageProcessor struct {
    amqpService     amqp.AMQPService
    s3Service       storage.S3Service
    redshiftService storage.RedshiftService
    logger          *zap.SugaredLogger
    s3OffloadLimit  int64
}

func NewMessageProcessor(amqpService amqp.AMQPService, s3Service storage.S3Service, redshiftService storage.RedshiftService, logger *zap.SugaredLogger, s3OffloadLimit int64) *MessageProcessor {
    return &MessageProcessor{
        amqpService:     amqpService,
        s3Service:       s3Service,
        redshiftService: redshiftService,
        logger:          logger,
        s3OffloadLimit:  s3OffloadLimit,
    }
}

func (mp *MessageProcessor) ProcessMessage(ctx context.Context, topic string, message *models.MessagePayload) error {
    // Convert message to JSON
    jsonMessage, err := json.Marshal(message)
    if err != nil {
        return fmt.Errorf("failed to marshal message: %w", err)
    }

    // Check if we need to offload to S3
    if mp.s3OffloadLimit == -1 || int64(len(jsonMessage)) > mp.s3OffloadLimit {
        // Store message content in S3
        s3Key := fmt.Sprintf("messages/%s/%s.json", topic, message.Timestamp)
        s3URI, err := mp.s3Service.UploadFile(ctx, s3Key, jsonMessage)
        if err != nil {
            return fmt.Errorf("failed to upload message to S3: %w", err)
        }
        
        // Update message with S3 URI and clear content
        message.S3URI = s3URI
        message.Content = nil
        
        // Re-marshal the updated message
        jsonMessage, err = json.Marshal(message)
        if err != nil {
            return fmt.Errorf("failed to marshal updated message: %w", err)
        }
    }

    // Store message metadata in Redshift
    query := fmt.Sprintf(`
        INSERT INTO messages (sender, timestamp, version, s3_uri)
        VALUES ('%s', '%s', '%s', '%s')
    `, message.Sender, message.Timestamp, message.Version, message.S3URI)
    
    err = mp.redshiftService.ExecuteQuery(ctx, query)
    if err != nil {
        return fmt.Errorf("failed to insert message metadata into Redshift: %w", err)
    }

    // Publish message to AMQP
    err = mp.amqpService.PublishMessage(ctx, topic, jsonMessage)
    if err != nil {
        return fmt.Errorf("failed to publish message to AMQP: %w", err)
    }

    mp.logger.Infow("Message processed successfully",
        "sender", message.Sender,
        "timestamp", message.Timestamp,
        "topic", topic,
        "s3URI", message.S3URI,
    )

    return nil
}
