package main

import (
    "bytes"
    "context"
    "encoding/json"
    "io"
    "net/http"
    "net/http/httptest"
    "os"
    "testing"
    "time"

    "github.com/aanthord/pubsub-amqp/internal/handlers"
    "github.com/aanthord/pubsub-amqp/internal/models"
    "github.com/gorilla/mux"
    "github.com/opentracing/opentracing-go"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/mock"
    "go.uber.org/zap"
)

var mockAMQPService *MockAMQPService
var mockTracer opentracing.Tracer
var logger *zap.SugaredLogger

// Mock AMQPService for isolated unit tests
type MockAMQPService struct {
    mock.Mock
}

func (m *MockAMQPService) PublishMessage(ctx context.Context, topic string, message *models.MessagePayload) error {
    args := m.Called(ctx, topic, message)
    return args.Error(0)
}

func (m *MockAMQPService) ConsumeMessage(ctx context.Context, topic string) (*models.MessagePayload, error) {
    args := m.Called(ctx, topic)
    return args.Get(0).(*models.MessagePayload), args.Error(1)
}

// Setup runs before each test
func setup() {
    // Initialize mock AMQP service and logger
    mockAMQPService = new(MockAMQPService)
    logger, _ = zap.NewDevelopment().Sugar()
}

// Test tracer initialization
func TestInitJaeger(t *testing.T) {
    os.Setenv("JAEGER_COLLECTOR_URL", "http://localhost:14268/api/traces")
    os.Setenv("JAEGER_FLUSH_INTERVAL", "1")

    tracer, closer, err := InitJaeger("pubsub-amqp")
    assert.NoError(t, err, "Jaeger initialization should not fail")
    assert.NotNil(t, tracer, "Tracer should not be nil")
    assert.NotNil(t, closer, "Closer should not be nil")

    closer.Close()
}

// Test publish handler success case
func TestPublishHandlerSuccess(t *testing.T) {
    setup()

    handler := handlers.NewPublishHandler(mockAMQPService, logger)

    messagePayload := models.NewMessagePayload("test-sender", map[string]interface{}{"key": "value"}, "")
    body, _ := json.Marshal(messagePayload)

    req := httptest.NewRequest("POST", "/api/v1/publish/test-topic", bytes.NewBuffer(body))
    req.Header.Set("Content-Type", "application/json")
    rec := httptest.NewRecorder()

    // Mock the PublishMessage function
    mockAMQPService.On("PublishMessage", mock.Anything, "test-topic", mock.Anything).Return(nil)

    handler.Handle(rec, req)

    assert.Equal(t, http.StatusAccepted, rec.Code, "Status code should be 202 Accepted")
    assert.JSONEq(t, `{"message":"Message published"}`, rec.Body.String(), "Response body should match")
}

// Test publish handler with invalid JSON body
func TestPublishHandlerInvalidBody(t *testing.T) {
    setup()

    handler := handlers.NewPublishHandler(mockAMQPService, logger)

    req := httptest.NewRequest("POST", "/api/v1/publish/test-topic", bytes.NewBuffer([]byte("invalid json")))
    req.Header.Set("Content-Type", "application/json")
    rec := httptest.NewRecorder()

    handler.Handle(rec, req)

    assert.Equal(t, http.StatusBadRequest, rec.Code, "Status code should be 400 Bad Request")
    assert.Contains(t, rec.Body.String(), "Invalid message format", "Response should contain format error")
}

// Test subscribe handler success case
func TestSubscribeHandlerSuccess(t *testing.T) {
    setup()

    handler := handlers.NewSubscribeHandler(mockAMQPService, logger)
    topic := "test-topic"

    // Mock message payload
    messagePayload := models.NewMessagePayload("test-sender", map[string]interface{}{"key": "value"}, "")
    mockAMQPService.On("ConsumeMessage", mock.Anything, topic).Return(messagePayload, nil)

    req := httptest.NewRequest("GET", "/api/v1/subscribe/test-topic", nil)
    rec := httptest.NewRecorder()

    handler.Handle(rec, req)

    assert.Equal(t, http.StatusOK, rec.Code, "Status code should be 200 OK")
    assert.Contains(t, rec.Body.String(), "key", "Response should contain message content")
}

// Test subscribe handler with no message available (204 No Content)
func TestSubscribeHandlerNoContent(t *testing.T) {
    setup()

    handler := handlers.NewSubscribeHandler(mockAMQPService, logger)
    topic := "test-topic"

    mockAMQPService.On("ConsumeMessage", mock.Anything, topic).Return(nil, nil)

    req := httptest.NewRequest("GET", "/api/v1/subscribe/test-topic", nil)
    rec := httptest.NewRecorder()

    handler.Handle(rec, req)

    assert.Equal(t, http.StatusNoContent, rec.Code, "Status code should be 204 No Content")
}

// Test AMQP tracing injection and extraction
func TestTracingAMQP(t *testing.T) {
    tracer, closer, err := InitJaeger("test-tracing")
    assert.NoError(t, err)
    opentracing.SetGlobalTracer(tracer)
    defer closer.Close()

    span := tracer.StartSpan("test-span")
    carrier := handlers.InjectTraceToAMQP(span)
    assert.NotEmpty(t, carrier, "Carrier should not be empty")

    spanContext, err := handlers.ExtractTraceFromAMQP(carrier)
    assert.NoError(t, err, "Span context extraction should not fail")
    assert.NotNil(t, spanContext, "Span context should not be nil")
}

// Test the health check endpoint
func TestHealthCheckHandler(t *testing.T) {
    req := httptest.NewRequest("GET", "/healthz", nil)
    rec := httptest.NewRecorder()

    healthCheckHandler(rec, req)

    assert.Equal(t, http.StatusOK, rec.Code, "Status code should be 200 OK")
    assert.Equal(t, "OK", rec.Body.String(), "Response should be OK")
}

// Test metrics endpoint
func TestMetricsHandler(t *testing.T) {
    req := httptest.NewRequest("GET", "/metrics", nil)
    rec := httptest.NewRecorder()

    http.DefaultServeMux.ServeHTTP(rec, req)

    assert.Equal(t, http.StatusOK, rec.Code, "Status code should be 200 OK")
}

// Test end-to-end simulation for publishing and subscribing
func TestEndToEndPublishSubscribe(t *testing.T) {
    setup()

    // Prepare the router with handlers
    router := mux.NewRouter()
    router.HandleFunc("/api/v1/publish/{topic}", handlers.NewPublishHandler(mockAMQPService, logger).Handle).Methods("POST")
    router.HandleFunc("/api/v1/subscribe/{topic}", handlers.NewSubscribeHandler(mockAMQPService, logger).Handle).Methods("GET")

    // Mock message payload
    messagePayload := models.NewMessagePayload("test-sender", map[string]interface{}{"key": "value"}, "")
    body, _ := json.Marshal(messagePayload)

    // Mock the PublishMessage and ConsumeMessage functions
    mockAMQPService.On("PublishMessage", mock.Anything, "test-topic", mock.Anything).Return(nil)
    mockAMQPService.On("ConsumeMessage", mock.Anything, "test-topic").Return(messagePayload, nil)

    // Test publish request
    req := httptest.NewRequest("POST", "/api/v1/publish/test-topic", bytes.NewBuffer(body))
    req.Header.Set("Content-Type", "application/json")
    rec := httptest.NewRecorder()
    router.ServeHTTP(rec, req)

    assert.Equal(t, http.StatusAccepted, rec.Code, "Publish: Status code should be 202 Accepted")

    // Test subscribe request
    req = httptest.NewRequest("GET", "/api/v1/subscribe/test-topic", nil)
    rec = httptest.NewRecorder()
    router.ServeHTTP(rec, req)

    assert.Equal(t, http.StatusOK, rec.Code, "Subscribe: Status code should be 200 OK")
    assert.Contains(t, rec.Body.String(), "key", "Subscribe: Response should contain message content")
}
