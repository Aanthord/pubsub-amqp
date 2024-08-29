package amqp

import (
    "context"
    "encoding/json"
    "encoding/xml"
    "fmt"

    "github.com/Azure/go-amqp"
    "github.com/aanthord/pubsub-amqp/internal/models"
    "github.com/aanthord/pubsub-amqp/internal/tracing"
    "go.uber.org/zap"
)

type AMQPService interface {
    PublishMessage(ctx context.Context, topic string, message *models.MessagePayload) error
    ConsumeMessage(ctx context.Context, topic string) (*models.MessagePayload, error) // Updated to match the single message consumption
}

type amqpService struct {
    client  *amqp.Conn
    session *amqp.Session
    logger  *zap.SugaredLogger
}

func NewAMQPService(amqpURL string, logger *zap.SugaredLogger) (AMQPService, error) {
    logger.Infow("Connecting to AMQP", "url", amqpURL)

    // Attempt to connect to the AMQP broker
    conn, err := amqp.Dial(context.TODO(), amqpURL, nil)
    if err != nil {
        logger.Errorw("Failed to connect to AMQP", "url", amqpURL, "error", err)
        return nil, fmt.Errorf("failed to connect to AMQP: %w", err)
    }
    logger.Info("Connected to AMQP successfully")

    // Start a session
    session, err := conn.NewSession(context.TODO(), nil)
    if err != nil {
        logger.Errorw("Failed to create AMQP session", "error", err)
        conn.Close() // Close the connection since we failed to create a session
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

// PublishMessage publishes a message to the specified topic
func (s *amqpService) PublishMessage(ctx context.Context, topic string, message *models.MessagePayload) error {
    // Start a new span for message publishing, passing the trace ID from the message
    span, ctx := tracing.StartSpanFromContext(ctx, "AMQPPublish", message.TraceID)
    defer span.Finish()

    // Marshal the message to JSON
    jsonMessage, err := json.Marshal(message)
    if err != nil {
        span.SetTag("error", true)
        span.LogKV("event", "marshal_error", "error.message", err.Error())
        s.logger.Errorw("Failed to marshal message", "error", err, "trace_id", message.TraceID)
        return fmt.Errorf("failed to marshal message: %w", err)
    }

    // Log the size of the marshaled message
    messageSize := len(jsonMessage)
    span.LogKV("event", "message_size", "message_size_bytes", messageSize, "trace_id", message.TraceID)
    s.logger.Infow("Marshaled message size", "message_size_bytes", messageSize, "trace_id", message.TraceID)

    // Create a new sender
    sender, err := s.session.NewSender(context.TODO(), topic, nil)
    if err != nil {
        span.SetTag("error", true)
        span.LogKV("event", "sender_error", "error.message", err.Error(), "trace_id", message.TraceID)
        s.logger.Errorw("Failed to create sender", "error", err, "topic", topic, "trace_id", message.TraceID)
        return fmt.Errorf("failed to create sender: %w", err)
    }
    defer func() {
        err := sender.Close(context.TODO())
        if err != nil {
            s.logger.Errorw("Failed to close sender", "error", err, "trace_id", message.TraceID)
        }
    }()

    // Prepare the message for sending, including the trace ID in the headers
    msg := amqp.NewMessage(jsonMessage)
    msg.ApplicationProperties = map[string]interface{}{
        "traceID": message.TraceID,
    }

    // Log the entire message before sending
    span.LogKV("event", "sending_message", "message_id", message.ID, "message_body", string(jsonMessage), "trace_id", message.TraceID)
    s.logger.Infow("Publishing message", "topic", topic, "message_id", message.ID, "trace_id", message.TraceID)

    // Send the message
    err = sender.Send(ctx, msg, nil)
    if err != nil {
        span.SetTag("error", true)
        span.LogKV("event", "publish_error", "error.message", err.Error(), "trace_id", message.TraceID)
        s.logger.Errorw("Failed to publish message", "error", err, "topic", topic, "message_id", message.ID, "trace_id", message.TraceID)
        return fmt.Errorf("failed to publish message: %w", err)
    }

    // Log successful message publication
    span.LogKV("event", "message_published", "message_id", message.ID, "trace_id", message.TraceID)
    s.logger.Infow("Message published successfully", "topic", topic, "message_id", message.ID, "trace_id", message.TraceID)

    return nil
}

// ConsumeMessage consumes a single message from the specified topic
// ConsumeMessage consumes a single message from the specified topic
func (s *amqpService) ConsumeMessage(ctx context.Context, topic string) (*models.MessagePayload, error) {
    // Create a new receiver
    receiver, err := s.session.NewReceiver(context.TODO(), topic, nil)
    if err != nil {
        s.logger.Errorw("Failed to create receiver", "error", err, "topic", topic)
        return nil, fmt.Errorf("failed to create receiver: %w", err)
    }
    defer func() {
        err := receiver.Close(context.TODO())
        if err != nil {
            s.logger.Errorw("Failed to close receiver", "error", err)
        }
    }()

    // Attempt to receive a single message
    msg, err := receiver.Receive(ctx, nil)
    if err != nil {
        if ctx.Err() == context.Canceled {
            s.logger.Info("Context canceled during message receive")
            return nil, ctx.Err()
        }

        s.logger.Errorw("Failed to receive message", "error", err)
        return nil, fmt.Errorf("failed to receive message: %w", err)
    }

    // Extract trace_id from message headers if available
    traceID, ok := msg.ApplicationProperties["traceID"].(string)
    if !ok || traceID == "" {
        traceID = "no-trace-id"
        s.logger.Warn("Message missing trace_id, using default")
    }

    // Start a new span for message processing using the extracted trace_id
    span, spanCtx := tracing.StartSpanFromContext(ctx, "AMQPConsume", traceID)
    defer span.Finish()

    // Log the raw message data size
    rawData := msg.GetData()
    dataSize := len(rawData)
    span.LogKV("event", "message_received", "message_size_bytes", dataSize, "trace_id", traceID)
    s.logger.Infow("Message received", "data_size", dataSize, "trace_id", traceID)

    // Unmarshal the message based on its Content-Type
    var payload models.MessagePayload
    contentType := msg.Properties.ContentType
    if contentType != nil && *contentType == "application/xml" {
        err = xml.Unmarshal(rawData, &payload)
    } else {
        err = json.Unmarshal(rawData, &payload)
    }

    if err != nil {
        span.SetTag("error", true)
        span.LogKV("event", "unmarshal_error", "error.message", err.Error(), "trace_id", traceID)
        s.logger.Errorw("Failed to unmarshal message", "error", err, "trace_id", traceID)
        return nil, fmt.Errorf("failed to unmarshal message: %w", err)
    }

    // Log the size of the unmarshaled message content
    if payload.Content != nil {
        contentSize := len(rawData) - len(payload.ID) - len(payload.TraceID) - len(payload.Sender) - len(payload.Timestamp) - len(payload.Version)
        span.LogKV("event", "message_content_size", "content_size_bytes", contentSize, "trace_id", traceID)
        s.logger.Infow("Unmarshaled message content size", "content_size", contentSize, "trace_id", traceID)
    }

    // Accept the message (acknowledge)
    err = receiver.AcceptMessage(spanCtx, msg)
    if err != nil {
        span.SetTag("error", true)
        span.LogKV("event", "accept_message_error", "error.message", err.Error(), "trace_id", traceID)
        s.logger.Errorw("Failed to accept message", "error", err, "trace_id", traceID)
        return nil, fmt.Errorf("failed to accept message: %w", err)
    }

    // Log the entire received message in the span and acknowledge successful processing
    span.LogKV("event", "message_processed", "message_id", payload.ID, "trace_id", payload.TraceID, "message_body", string(rawData))
    s.logger.Infow("Message processed and acknowledged", "message_id", payload.ID, "trace_id", payload.TraceID)

    return &payload, nil
}

