package main

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"log"
	"math"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/gorilla/mux"
	"github.com/joho/godotenv"
	"github.com/opentracing/opentracing-go"
	opentracinglog "github.com/opentracing/opentracing-go/log"
	"github.com/rs/cors"
	httpSwagger "github.com/swaggo/http-swagger"
	"github.com/uber/jaeger-client-go"
	jaegerconfig "github.com/uber/jaeger-client-go/config"
	"github.com/uber/jaeger-lib/metrics"
	"pack.ag/amqp"

	// Import generated docs
	"github.com/aanthord/pubsub-ampq/docs"
)

// MessagePayload defines the structure of the AMQP message payload
type MessagePayload struct {
	XMLName   xml.Name               `xml:"OAGIMessage" json:"-"`
	Sender    string                 `xml:"Sender,attr" json:"sender"`
	Timestamp string                 `xml:"Timestamp,attr" json:"timestamp"`
	Version   string                 `xml:"Version,attr" json:"version"`
	Content   map[string]interface{} `xml:",any" json:"content"`
	S3URI     string                 `xml:"S3URI,omitempty" json:"s3_uri,omitempty"`
}

// MarshalXML implements custom XML marshaling for MessagePayload
func (m MessagePayload) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	start.Name = xml.Name{Local: "OAGIMessage"}
	start.Attr = []xml.Attr{
		{Name: xml.Name{Local: "Sender"}, Value: m.Sender},
		{Name: xml.Name{Local: "Timestamp"}, Value: m.Timestamp},
		{Name: xml.Name{Local: "Version"}, Value: m.Version},
	}
	if err := e.EncodeToken(start); err != nil {
		return err
	}
	for k, v := range m.Content {
		if err := e.EncodeElement(v, xml.StartElement{Name: xml.Name{Local: k}}); err != nil {
			return err
		}
	}
	return e.EncodeToken(start.End())
}

// UnmarshalXML implements custom XML unmarshaling for MessagePayload
func (m *MessagePayload) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	m.Content = make(map[string]interface{})
	for _, attr := range start.Attr {
		switch attr.Name.Local {
		case "Sender":
			m.Sender = attr.Value
		case "Timestamp":
			m.Timestamp = attr.Value
		case "Version":
			m.Version = attr.Value
		}
	}
	for {
		token, err := d.Token()
		if err != nil {
			return err
		}
		switch se := token.(type) {
		case xml.StartElement:
			var value interface{}
			if err := d.DecodeElement(&value, &se); err != nil {
				return err
			}
			m.Content[se.Name.Local] = value
		case xml.EndElement:
			if se.Name == start.Name {
				return nil
			}
		}
	}
}

// AmqpService handles AMQP operations
type AmqpService struct {
	client *amqp.Client
	s3svc  *s3.Client
}

// NewAmqpService creates a new AMQP service with S3 integration
func NewAmqpService(ctx context.Context) (*AmqpService, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "NewAmqpService")
	defer span.Finish()

	amqpURL := os.Getenv("AMQP_URL")
	if amqpURL == "" {
		err := fmt.Errorf("AMQP_URL environment variable not set")
		span.LogFields(opentracinglog.Error(err))
		return nil, err
	}

	client, err := amqp.Dial(amqpURL)
	if err != nil {
		span.LogFields(opentracinglog.Error(err))
		return nil, fmt.Errorf("failed to connect to AMQP at %s: %w", amqpURL, err)
	}

	// Load the default configuration from the environment and shared config
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(os.Getenv("AWS_REGION")))
	if err != nil {
		span.LogFields(opentracinglog.Error(err))
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	s3svc := s3.NewFromConfig(cfg)

	return &AmqpService{client: client, s3svc: s3svc}, nil
}

// SaveToS3 saves large content to S3 and returns the S3 URI
func (s *AmqpService) SaveToS3(ctx context.Context, content []byte) (string, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "SaveToS3")
	defer span.Finish()

	bucket := os.Getenv("S3_BUCKET_NAME")
	key := fmt.Sprintf("messages/%s.xml", time.Now().Format("20060102150405"))

	_, err := s.s3svc.PutObject(ctx, &s3.PutObjectInput{
		Bucket: &bucket,
		Key:    &key,
		Body:   bytes.NewReader(content),
	})
	if err != nil {
		span.LogFields(opentracinglog.Error(err))
		return "", fmt.Errorf("failed to upload to S3: %w", err)
	}

	s3URI := fmt.Sprintf("s3://%s/%s", bucket, key)
	span.LogFields(opentracinglog.String("s3_uri", s3URI))
	return s3URI, nil
}

// PublishMessage publishes a message to a topic with retry and exponential backoff
func (s *AmqpService) PublishMessage(ctx context.Context, topic string, messagePayload MessagePayload) error {
	span, ctx := opentracing.StartSpanFromContext(ctx, "PublishMessage")
	defer span.Finish()

	span.SetTag("topic", topic)

	data, err := xml.MarshalIndent(messagePayload, "", "  ")
	if err != nil {
		span.LogFields(opentracinglog.Error(err))
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// Check if the message is too large (over 4MB)
	if len(data) > 4*1024*1024 {
		s3URI, err := s.SaveToS3(ctx, data)
		if err != nil {
			span.LogFields(opentracinglog.Error(err))
			return fmt.Errorf("failed to save large message to S3: %w", err)
		}
		messagePayload.S3URI = s3URI
		messagePayload.Content = nil // Clear the content as it's now in S3
		data, err = xml.MarshalIndent(messagePayload, "", "  ")
		if err != nil {
			span.LogFields(opentracinglog.Error(err))
			return fmt.Errorf("failed to marshal message with S3 URI: %w", err)
		}
	}

	maxRetries := getEnvInt("MAX_RETRIES", 3)
	initialBackoffMs := getEnvInt("INITIAL_BACKOFF_MS", 100)
	maxBackoffMs := getEnvInt("MAX_BACKOFF_MS", 1000)

	for attempt := 0; attempt <= maxRetries; attempt++ {
		session, err := s.client.NewSession()
		if err != nil {
			span.LogFields(opentracinglog.Error(err))
			continue
		}
		defer session.Close(ctx)

		sender, err := session.NewSender(
			amqp.LinkTargetAddress(topic),
		)
		if err != nil {
			span.LogFields(opentracinglog.Error(err))
			continue
		}
		defer sender.Close(ctx)

		err = sender.Send(ctx, amqp.NewMessage(data))
		if err == nil {
			span.LogFields(
				opentracinglog.String("message", string(data)),
				opentracinglog.Int("attempt", attempt),
			)
			return nil
		}

		backoff := time.Duration(math.Min(float64(initialBackoffMs)*math.Pow(2, float64(attempt)), float64(maxBackoffMs))) * time.Millisecond
		backoff = time.Duration(rand.Int63n(int64(backoff)))
		span.LogFields(
			opentracinglog.Int("attempt", attempt),
			opentracinglog.String("backoff", backoff.String()),
		)
		time.Sleep(backoff)
	}

	span.LogFields(opentracinglog.Error(fmt.Errorf("max retries reached")))
	return fmt.Errorf("failed to publish message to topic %s after %d attempts", topic, maxRetries)
}

// ReceiveMessage subscribes to a topic and receives messages with retry and exponential backoff
func (s *AmqpService) ReceiveMessage(ctx context.Context, topic string) (*MessagePayload, error) {
	span, ctx := opentracing.StartSpanFromContext(ctx, "ReceiveMessage")
	defer span.Finish()

	span.SetTag("topic", topic)

	maxRetries := getEnvInt("MAX_RETRIES", 3)
	initialBackoffMs := getEnvInt("INITIAL_BACKOFF_MS", 100)
	maxBackoffMs := getEnvInt("MAX_BACKOFF_MS", 1000)

	for attempt := 0; attempt <= maxRetries; attempt++ {
		session, err := s.client.NewSession()
		if err != nil {
			span.LogFields(opentracinglog.Error(err))
			continue
		}
		defer session.Close(ctx)

		receiver, err := session.NewReceiver(
			amqp.LinkSourceAddress(topic),
		)
		if err != nil {
			span.LogFields(opentracinglog.Error(err))
			continue
		}
		defer receiver.Close(ctx)

		msg, err := receiver.Receive(ctx)
		if err == nil {
			var messagePayload MessagePayload
			err = xml.Unmarshal(msg.GetData(), &messagePayload)
			if err == nil {
				span.LogFields(
					opentracinglog.String("message", fmt.Sprintf("%+v", messagePayload)),
					opentracinglog.Int("attempt", attempt),
				)
				return &messagePayload, nil
			}
		}

		backoff := time.Duration(math.Min(float64(initialBackoffMs)*math.Pow(2, float64(attempt)), float64(maxBackoffMs))) * time.Millisecond
		backoff = time.Duration(rand.Int63n(int64(backoff)))
		span.LogFields(
			opentracinglog.Int("attempt", attempt),
			opentracinglog.String("backoff", backoff.String()),
		)
		time.Sleep(backoff)
	}

	span.LogFields(opentracinglog.Error(fmt.Errorf("max retries reached")))
	return nil, fmt.Errorf("failed to receive message from topic %s after %d attempts", topic, maxRetries)
}

// InitJaeger initializes a Jaeger tracer
func InitJaeger(serviceName string) (opentracing.Tracer, func(), error) {
	cfg := &jaegerconfig.Configuration{
		ServiceName: serviceName,
		Sampler: &jaegerconfig.SamplerConfig{
			Type:  jaeger.SamplerTypeConst,
			Param: 1,
		},
		Reporter: &jaegerconfig.ReporterConfig{
			LogSpans:           true,
			LocalAgentHostPort: os.Getenv("JAEGER_AGENT_HOST") + ":" + os.Getenv("JAEGER_AGENT_PORT"),
		},
	}

	tracer, closer, err := cfg.NewTracer(
		jaegerconfig.Logger(jaeger.StdLogger),
		jaegerconfig.Metrics(metrics.NullFactory),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("could not initialize Jaeger Tracer: %w", err)
	}

	return tracer, func() {
		closer.Close()
	}, nil
}

// PublishHandler handles publishing messages to a topic
// @Summary Publish a message to a topic
// @Description Publish a message to a specified topic
// @Tags messaging
// @Accept  json,xml
// @Produce  plain
// @Param topic query string true "Topic"
// @Param message body MessagePayload true "Message Payload"
// @Success 200 {string} string "Message published"
// @Failure 400 {string} string "Invalid payload"
// @Failure 500 {string} string "Failed to publish message"
// @Router /api/v1/publish [post]
func PublishHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("Received request to publish message")
	span, ctx := opentracing.StartSpanFromContext(r.Context(), "PublishHandler")
	defer span.Finish()

	topic := r.URL.Query().Get("topic")
	span.SetTag("topic", topic)
	log.Printf("Publishing to topic: %s", topic)

	var messagePayload MessagePayload
	contentType := r.Header.Get("Content-Type")

	switch contentType {
	case "application/json":
		if err := json.NewDecoder(r.Body).Decode(&messagePayload); err != nil {
			span.LogFields(opentracinglog.Error(err))
			log.Printf("Error decoding JSON: %v", err)
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}
	case "application/xml":
		if err := xml.NewDecoder(r.Body).Decode(&messagePayload); err != nil {
			span.LogFields(opentracinglog.Error(err))
			log.Printf("Error decoding XML: %v", err)
			http.Error(w, "Invalid XML", http.StatusBadRequest)
			return
		}
	default:
		span.LogFields(opentracinglog.String("error", "Unsupported Content-Type"))
		log.Printf("Unsupported Content-Type: %s", contentType)
		http.Error(w, "Unsupported Content-Type", http.StatusBadRequest)
		return
	}

	if messagePayload.Timestamp == "" {
		messagePayload.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}

	amqpService, err := NewAmqpService(ctx)
	if err != nil {
		span.LogFields(opentracinglog.Error(err))
		log.Printf("Error creating AMQP service: %v", err)
		http.Error(w, "Failed to create AMQP service", http.StatusInternalServerError)
		return
	}

	if err := amqpService.PublishMessage(ctx, topic, messagePayload); err != nil {
		span.LogFields(opentracinglog.Error(err))
		log.Printf("Error publishing message: %v", err)
		http.Error(w, "Failed to publish message", http.StatusInternalServerError)
		return
	}

	span.LogFields(opentracinglog.String("message", fmt.Sprintf("%+v", messagePayload)))
	log.Println("Message published successfully")

	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, "Message published")
}

// SubscribeHandler handles subscribing to a topic
// @Summary Subscribe to a topic
// @Description Subscribe to a specified topic and receive messages
// @Tags messaging
// @Accept  plain
// @Produce  xml
// @Param topic query string true "Topic"
// @Success 200 {object} MessagePayload "Received message"
// @Failure 500 {string} string "Failed to receive message"
// @Router /api/v1/subscribe [get]
func SubscribeHandler(w http.ResponseWriter, r *http.Request) {
	span, ctx := opentracing.StartSpanFromContext(r.Context(), "SubscribeHandler")
	defer span.Finish()

	topic := r.URL.Query().Get("topic")
	span.SetTag("topic", topic)

	amqpService, err := NewAmqpService(ctx)
	if err != nil {
		span.LogFields(opentracinglog.Error(err))
		http.Error(w, "Failed to create AMQP service", http.StatusInternalServerError)
		return
	}

	message, err := amqpService.ReceiveMessage(ctx, topic)
	if err != nil {
		span.LogFields(opentracinglog.Error(err))
		http.Error(w, "Failed to receive message", http.StatusInternalServerError)
		return
	}

	span.LogFields(opentracinglog.String("message", fmt.Sprintf("%+v", message)))

	w.Header().Set("Content-Type", "application/xml")
	xml.NewEncoder(w).Encode(message)
}

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("Error loading .env file, using environment variables")
	}

	tracer, closer, err := InitJaeger("AMQP-Service")
	if err != nil {
		log.Fatalf("Could not initialize Jaeger Tracer: %s", err)
	}
	defer closer()

	opentracing.SetGlobalTracer(tracer)

	// Set up Swagger
	swaggerInfo := docs.SwaggerInfo
	swaggerInfo.Title = "AMQP Pub/Sub API"
	swaggerInfo.Description = "API for publishing and subscribing to AMQP topics"
	swaggerInfo.Version = "1.0"
	swaggerInfo.Host = "txanunxlbapd512:8080"
	swaggerInfo.BasePath = "/api/v1"
	swaggerInfo.Schemes = []string{"http", "https"}

	router := mux.NewRouter()

	// API routes
	apiRouter := router.PathPrefix("/api/v1").Subrouter()
	apiRouter.HandleFunc("/publish", PublishHandler).Methods("POST")
	apiRouter.HandleFunc("/subscribe", SubscribeHandler).Methods("GET")

	// Swagger UI
	router.PathPrefix("/swagger/").Handler(httpSwagger.WrapHandler)

	// Initialize CORS middleware with your desired settings
	corsHandler := cors.New(cors.Options{
		AllowedOrigins: []string{"http://txanunxlbapd512", "http://txanunxlbapd512.goldlnk.rootlnka.net"}, // Include your server's DNS name
		AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{
			"Content-Type",
			"Authorization",
			"Accept",
			"X-Requested-With",
			"application/json",
			"application/xml",
			"text/xml",
			"multipart/form-data",
			"application/x-www-form-urlencoded",
			"text/plain",
			"text/html",
			"application/javascript",
			"text/css",
			"image/png",
			"image/jpeg",
			"image/gif",
			"image/webp",
			"application/octet-stream",
		},
		ExposedHeaders:   []string{"Content-Length"},
		AllowCredentials: true,
	})

	// Wrap your router with the CORS middleware
	handler := corsHandler.Handler(router)

	// Start the server
	log.Println("Server starting on :8080")
	log.Fatal(http.ListenAndServe(":8080", handler))
}

// getEnvInt reads an integer from the environment or returns a default value if the variable is not set or invalid
func getEnvInt(key string, defaultValue int) int {
	if valueStr, exists := os.LookupEnv(key); exists {
		if value, err := strconv.Atoi(valueStr); err == nil {
			return value
		}
	}
	return defaultValue
}
