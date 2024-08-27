package amqp

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/aanthord/pubsub-amqp/internal/config"
	"github.com/aanthord/pubsub-amqp/internal/metrics"
	"github.com/streadway/amqp"
	"go.uber.org/zap"
)

type AMQPService interface {
	PublishMessage(ctx context.Context, topic string, message []byte) error
	SubscribeToTopic(ctx context.Context, topic string) (<-chan []byte, error)
}

type amqpService struct {
	conn   *amqp.Connection
	logger *zap.SugaredLogger
}

func NewAMQPService() (AMQPService, error) {
	logger, _ := zap.NewProduction()
	sugar := logger.Sugar()

	amqpURL := config.GetEnv("AMQP_URL", "amqp://guest:guest@localhost:5672/")
	conn, err := amqp.Dial(amqpURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to AMQP: %w", err)
	}

	return &amqpService{conn: conn, logger: sugar}, nil
}

func (s *amqpService) PublishMessage(ctx context.Context, topic string, message []byte) error {
	start := time.Now()
	defer func() {
		metrics.MessagePublishDuration.Observe(time.Since(start).Seconds())
	}()

	ch, err := s.conn.Channel()
	if err != nil {
		return fmt.Errorf("failed to open a channel: %w", err)
	}
	defer ch.Close()

	err = ch.ExchangeDeclare(
		topic,   // name
		"topic", // type
		true,    // durable
		false,   // auto-deleted
		false,   // internal
		false,   // no-wait
		nil,     // arguments
	)
	if err != nil {
		return fmt.Errorf("failed to declare an exchange: %w", err)
	}

	err = ch.Publish(
		topic, // exchange
		"",    // routing key
		false, // mandatory
		false, // immediate
		amqp.Publishing{
			ContentType: "text/plain",
			Body:        message,
		})
	if err != nil {
		return fmt.Errorf("failed to publish a message: %w", err)
	}

	metrics.MessagesPublished.Inc()
	s.logger.Infow("Message published", "topic", topic, "size", len(message))
	return nil
}

func (s *amqpService) SubscribeToTopic(ctx context.Context, topic string) (<-chan []byte, error) {
	ch, err := s.conn.Channel()
	if err != nil {
		return nil, fmt.Errorf("failed to open a channel: %w", err)
	}

	err = ch.ExchangeDeclare(
		topic,   // name
		"topic", // type
		true,    // durable
		false,   // auto-deleted
		false,   // internal
		false,   // no-wait
		nil,     // arguments
	)
	if err != nil {
		return nil, fmt.Errorf("failed to declare an exchange: %w", err)
	}

	q, err := ch.QueueDeclare(
		"",    // name
		false, // durable
		false, // delete when unused
		true,  // exclusive
		false, // no-wait
		nil,   // arguments
	)
	if err != nil {
		return nil, fmt.Errorf("failed to declare a queue: %w", err)
	}

	err = ch.QueueBind(
		q.Name, // queue name
		"",     // routing key
		topic,  // exchange
		false,
		nil)
	if err != nil {
		return nil, fmt.Errorf("failed to bind a queue: %w", err)
	}

	msgs, err := ch.Consume(
		q.Name, // queue
		"",     // consumer
		true,   // auto-ack
		false,  // exclusive
		false,  // no-local
		false,  // no-wait
		nil,    // args
	)
	if err != nil {
		return nil, fmt.Errorf("failed to register a consumer: %w", err)
	}

	messageChan := make(chan []byte)
	go func() {
		for d := range msgs {
			select {
			case messageChan <- d.Body:
				metrics.MessagesReceived.Inc()
				s.logger.Infow("Message received", "topic", topic, "size", len(d.Body))
			case <-ctx.Done():
				return
			}
		}
	}()

	return messageChan, nil
}
