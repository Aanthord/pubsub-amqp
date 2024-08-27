package amqp

import (
    "context"
    "encoding/json"
    "fmt"

    "github.com/aanthord/pubsub-amqp/internal/models"
    "github.com/aanthord/pubsub-amqp/internal/tracing"
    "github.com/streadway/amqp"
    "github.com/opentracing/opentracing-go"
    "go.uber.org/zap"
)

type AMQPService interface {
    PublishMessage(ctx context.Context, topic string, message *models.MessagePayload) error
    ConsumeMessages(ctx context.Context, topic string) (<-chan *models.MessagePayload, error)
}

type amqpService struct {
    conn    *amqp.Connection
    channel *amqp.Channel
    logger  *zap.SugaredLogger
}

func NewAMQPService(amqpURL string, logger *zap.SugaredLogger) (AMQPService, error) {
    conn, err := amqp.Dial(amqpURL)
    if err != nil {
        return nil, fmt.Errorf("failed to connect to AMQP: %w", err)
    }

    channel, err := conn.Channel()
    if err != nil {
        return nil, fmt.Errorf("failed to open a channel: %w", err)
    }

    return &amqpService{
        conn:    conn,
        channel: channel,
        logger:  logger,
    }, nil
}

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

    headers := amqp.Table{}
    for k, v := range traceHeaders {
        headers[k] = v
    }

    err = s.channel.Publish(
        topic,
        "",
        false,
        false,
        amqp.Publishing{
            ContentType: "application/json",
            Body:        jsonMessage,
            Headers:     headers,
        },
    )

    if err != nil {
        span.SetTag("error", true)
        span.LogKV("event", "publish_error", "error.message", err.Error())
        return fmt.Errorf("failed to publish message: %w", err)
    }

    span.LogKV("event", "message_published", "message_id", message.ID)
    return nil
}

func (s *amqpService) ConsumeMessages(ctx context.Context, topic string) (<-chan *models.MessagePayload, error) {
    span, _ := tracing.StartSpanFromContext(ctx, "AMQPConsume", "")
    defer span.Finish()

    msgs, err := s.channel.Consume(
        topic,
        "",
        true,
        false,
        false,
        false,
        nil,
    )
    if err != nil {
        span.SetTag("error", true)
        span.LogKV("event", "consume_error", "error.message", err.Error())
        return nil, fmt.Errorf("failed to consume messages: %w", err)
    }

    out := make(chan *models.MessagePayload)
    go func() {
        for msg := range msgs {
            spanContext, _ := tracing.ExtractTraceFromAMQP(msg.Headers)
            childSpan := opentracing.StartSpan(
                "ProcessAMQPMessage",
                opentracing.ChildOf(spanContext),
            )

            var payload models.MessagePayload
            err := json.Unmarshal(msg.Body, &payload)
            if err != nil {
                childSpan.SetTag("error", true)
                childSpan.LogKV("event", "unmarshal_error", "error.message", err.Error())
                childSpan.Finish()
                continue
            }

            childSpan.LogKV("event", "message_received", "message_id", payload.ID, "trace_id", payload.TraceID)
            out <- &payload
            childSpan.Finish()
        }
        close(out)
    }()

    return out, nil
}
