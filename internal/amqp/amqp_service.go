package amqp

import (
    "context"
    "encoding/json"
    "fmt"

    "github.com/Azure/go-amqp"
    "github.com/aanthord/pubsub-amqp/internal/models"
    "github.com/aanthord/pubsub-amqp/internal/tracing"
    "go.uber.org/zap"
)

type AMQPService interface {
    PublishMessage(ctx context.Context, queueName string, message *models.MessagePayload) error
    ConsumeMessage(ctx context.Context, queueName string) (*models.MessagePayload, error)
}

type amqpService struct {
    client  *amqp.Conn
    session *amqp.Session
    logger  *zap.SugaredLogger
}

// NewAMQPService creates a new AMQPService instance
func NewAMQPService(amqpURL string, logger *zap.SugaredLogger) (AMQPService, error) {
    logger.Infow("Connecting to AMQP", "url", amqpURL)

    // Connect to the AMQP broker
    conn, err := amqp.Dial(context.TODO(), amqpURL, nil)
    if err != nil {
        logger.Errorw("Failed to connect to AMQP", "url", amqpURL, "error", err)
        return nil, fmt.Errorf("failed to connect to AMQP: %w", err)
    }
    logger.Info("Connected to AMQP successfully")

    // Create a new session
    session, err := conn.NewSession(context.TODO(), nil)
    if err != nil {
        logger.Errorw("Failed to create AMQP session", "error", err)
        conn.Close()
        return nil, fmt.Errorf("failed to create session: %w", err)
    }
    logger.Info("Opened AMQP session successfully")

    // Return the AMQP service instance
    return &amqpService{
        client:  conn,
        session: session,
        logger:  logger,
    }, nil
}

// PublishMessage publishes a message to the specified anycast queue
func (s *amqpService) PublishMessage(ctx context.Context, queueName string, message *models.MessagePayload) error {
    span, ctx := tracing.StartSpanFromContext(ctx, "AMQPPublish", message.Header.TraceID)
    defer span.Finish()

    // Marshal the message to JSON
    jsonMessage, err := json.Marshal(message)
    if err != nil {
        span.SetTag("error", true)
        span.LogKV("event", "marshal_error", "error.message", err.Error())
        s.logger.Errorw("Failed to marshal message", "error", err, "trace_id", message.Header.TraceID)
        return fmt.Errorf("failed to marshal message: %w", err)
    }

    // Ensure the queue exists
    err = s.ensureQueueExists(ctx, queueName)
    if err != nil {
        s.logger.Errorw("Failed to ensure queue exists", "queueName", queueName, "error", err)
        return fmt.Errorf("failed to ensure queue exists: %w", err)
    }

    // Create a sender
    sender, err := s.session.NewSender(context.TODO(), queueName, &amqp.SenderOptions{
        Target: &amqp.Target{
            Address:      queueName,
            Durable:      amqp.Durable,
            Capabilities: []amqp.Symbol{"anycast"},
        },
    })
    if err != nil {
        s.logger.Errorw("Failed to create sender", "error", err, "queue", queueName)
        return fmt.Errorf("failed to create sender: %w", err)
    }
    defer sender.Close(ctx)

    // Prepare the message
    msg := amqp.NewMessage(jsonMessage)
    msg.ApplicationProperties = map[string]interface{}{
        "traceID": message.Header.TraceID,
    }

    // Send the message
    err = sender.Send(ctx, msg, nil)
    if err != nil {
        s.logger.Errorw("Failed to publish message", "error", err, "queue", queueName)
        return fmt.Errorf("failed to publish message: %w", err)
    }

    s.logger.Infow("Message published successfully", "queue", queueName, "message_id", message.Header.ID)
    return nil
}

// ConsumeMessage consumes a single message from the specified anycast queue
func (s *amqpService) ConsumeMessage(ctx context.Context, queueName string) (*models.MessagePayload, error) {
    // Create a receiver for the anycast queue
    receiver, err := s.session.NewReceiver(context.TODO(), queueName, &amqp.ReceiverOptions{
        Source: &amqp.Source{
            Address:      queueName,
            Capabilities: []amqp.Symbol{"anycast"},
        },
    })
    if err != nil {
        s.logger.Errorw("Failed to create receiver", "error", err, "queue", queueName)
        return nil, fmt.Errorf("failed to create receiver: %w", err)
    }
    defer receiver.Close(ctx)

    // Receive a message
    msg, err := receiver.Receive(ctx, nil)
    if err != nil {
        if ctx.Err() == context.Canceled {
            s.logger.Info("Context canceled during message receive")
            return nil, ctx.Err()
        }
        s.logger.Errorw("Failed to receive message", "error", err, "queue", queueName)
        return nil, fmt.Errorf("failed to receive message: %w", err)
    }

    // Extract traceID from the message
    traceID, ok := msg.ApplicationProperties["traceID"].(string)
    if !ok {
        traceID = "no-trace-id"
        s.logger.Warn("Received message missing traceID")
    }

    // Start a span and process the message
    span, spanCtx := tracing.StartSpanFromContext(ctx, "AMQPConsume", traceID)
    defer span.Finish()

    // Unmarshal the message
    var payload models.MessagePayload
    if err := json.Unmarshal(msg.GetData(), &payload); err != nil {
        s.logger.Errorw("Failed to unmarshal message", "error", err, "traceID", traceID)
        span.SetTag("error", true)
        span.LogKV("event", "unmarshal_error", "error.message", err.Error())
        return nil, fmt.Errorf("failed to unmarshal message: %w", err)
    }

    // Log the payload into the span
    span.LogKV("event", "message_received", "message_id", payload.Header.ID, "payload", string(msg.GetData()))

    // Acknowledge the message
    if err := receiver.AcceptMessage(spanCtx, msg); err != nil {
        s.logger.Errorw("Failed to acknowledge message", "error", err, "traceID", traceID)
        span.SetTag("error", true)
        span.LogKV("event", "ack_error", "error.message", err.Error())
        return nil, fmt.Errorf("failed to accept message: %w", err)
    }

    s.logger.Infow("Message consumed and acknowledged", "queue", queueName, "message_id", payload.Header.ID)
    return &payload, nil
}

// ensureQueueExists ensures the queue is created if it doesn't already exist
func (s *amqpService) ensureQueueExists(ctx context.Context, queueName string) error {
    sender, err := s.session.NewSender(ctx, queueName, &amqp.SenderOptions{
        Target: &amqp.Target{
            Address:      queueName,
            Durable:      amqp.Durable,
            Capabilities: []amqp.Symbol{"anycast"},
        },
    })
    if err != nil {
        s.logger.Errorw("Failed to ensure queue exists", "error", err, "queue", queueName)
        return fmt.Errorf("failed to create queue: %w", err)
    }
    defer sender.Close(ctx)
    return nil
}
