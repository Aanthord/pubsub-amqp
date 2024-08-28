package amqp

import (
    "context"
    "encoding/json"
    "fmt"

    "github.com/aanthord/pubsub-amqp/internal/models"
    "github.com/aanthord/pubsub-amqp/internal/tracing"
    "pack.ag/amqp"

    "go.uber.org/zap"
)

type AMQPService interface {
    PublishMessage(ctx context.Context, topic string, message *models.MessagePayload) error
    ConsumeMessages(ctx context.Context, topic string) (<-chan *models.MessagePayload, error)
}

type amqpService struct {
    client  *amqp.Client
    session *amqp.Session
    logger  *zap.SugaredLogger
}

func NewAMQPService(amqpURL string, logger *zap.SugaredLogger) (AMQPService, error) {
    logger.Infow("Connecting to AMQP", "url", amqpURL)

    // Attempt to connect to the AMQP broker
    client, err := amqp.Dial(amqpURL)
    if err != nil {
        logger.Errorw("Failed to connect to AMQP", "url", amqpURL, "error", err)
        return nil, fmt.Errorf("failed to connect to AMQP: %w", err)
    }
    logger.Info("Connected to AMQP successfully")

    // Start a session
    session, err := client.NewSession()
    if err != nil {
        logger.Errorw("Failed to create AMQP session", "error", err)
        client.Close() // Close the client since we failed to create a session
        return nil, fmt.Errorf("failed to create session: %w", err)
    }
    logger.Info("Opened AMQP session successfully")

    // Return the AMQP service instance
    return &amqpService{
        client:  client,
        session: session,
        logger:  logger,
    }, nil
}

// PublishMessage publishes a message to the specified topic
func (s *amqpService) PublishMessage(ctx context.Context, topic string, message *models.MessagePayload) error {
    span, _ := tracing.StartSpanFromContext(ctx, "AMQPPublish", message.TraceID)
    defer span.Finish()

    traceHeaders := tracing.InjectTraceToAMQP(span)

    jsonMessage, err := json.Marshal(message)
    if err != nil {
        span.SetTag("error", true)
        span.LogKV("event", "marshal_error", "error.message", err.Error())
        return fmt.Errorf("failed to marshal message: %w", err)
    }

    sender, err := s.session.NewSender(
        amqp.LinkTargetAddress(topic),
        amqp.LinkTargetDurability(amqp.DurabilityUnsettledState), // Ensures the target (queue) is durable
        amqp.LinkTargetExpiryPolicy(amqp.ExpiryNever),
    )
    if err != nil {
        span.SetTag("error", true)
        span.LogKV("event", "sender_error", "error.message", err.Error())
        return fmt.Errorf("failed to create sender: %w", err)
    }
    defer sender.Close(ctx)

    msg := amqp.NewMessage(jsonMessage)
    msg.ApplicationProperties = make(map[string]interface{})
    for k, v := range traceHeaders {
        msg.ApplicationProperties[k] = v
    }

    s.logger.Infow("Publishing message", "topic", topic, "message_id", message.ID)
    err = sender.Send(ctx, msg)
    if err != nil {
        span.SetTag("error", true)
        span.LogKV("event", "publish_error", "error.message", err.Error())
        s.logger.Errorw("Failed to publish message", "error", err, "topic", topic, "message_id", message.ID)
        return fmt.Errorf("failed to publish message: %w", err)
    }

    span.LogKV("event", "message_published", "message_id", message.ID)
    s.logger.Infow("Message published successfully", "topic", topic, "message_id", message.ID)
    return nil
}

// ConsumeMessages consumes a single message from the specified topic
func (s *amqpService) ConsumeMessages(ctx context.Context, topic string) (<-chan *models.MessagePayload, error) {
    out := make(chan *models.MessagePayload)

    go func() {
        defer close(out)

        receiver, err := s.session.NewReceiver(
            amqp.LinkSourceAddress(topic),
            amqp.LinkCredit(1), // Prefetch only 1 message at a time
        )
        if err != nil {
            s.logger.Errorw("Failed to create receiver", "error", err, "topic", topic)
            return
        }
        defer receiver.Close(ctx)

        msg, err := receiver.Receive(ctx)
        if err != nil {
            s.logger.Errorw("Failed to receive message", "error", err)
            return
        }
        defer msg.Accept() // Acknowledge the message

        var payload models.MessagePayload
        err = json.Unmarshal(msg.GetData(), &payload)
        if err != nil {
            s.logger.Errorw("Failed to unmarshal message", "error", err)
            return
        }

        s.logger.Infow("Message received", "message_id", payload.ID, "trace_id", payload.TraceID)
        out <- &payload
    }()

    return out, nil
}
