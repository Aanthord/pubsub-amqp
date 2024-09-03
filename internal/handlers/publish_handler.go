package handlers

import (
    "encoding/json"
    "io/ioutil"
    "net/http"
    "time"
    "bytes"
    "github.com/aanthord/pubsub-amqp/internal/amqp"
    "github.com/aanthord/pubsub-amqp/internal/metrics"
    "github.com/aanthord/pubsub-amqp/internal/models"
    "github.com/aanthord/pubsub-amqp/internal/tracing"
    "github.com/gorilla/mux"
    "go.uber.org/zap"
)

// SuccessResponse represents a generic success response
type SuccessResponse struct {
    Message string `json:"message"`
}

// PublishHandler handles publishing messages to a topic.
type PublishHandler struct {
    service amqp.AMQPService
    logger  *zap.SugaredLogger
}

func NewPublishHandler(service amqp.AMQPService, logger *zap.SugaredLogger) *PublishHandler {
    return &PublishHandler{service: service, logger: logger}
}

// @Summary Publish a message to a topic
// @Description Publishes a message to the specified AMQP topic
// @Tags messages
// @Accept json
// @Produce json
// @Param topic path string true "Topic to publish to"
// @Param message body models.MessagePayload true "Message payload"
// @Success 202 {object} SuccessResponse "Message published successfully"
// @Failure 400 {object} ErrorResponse "Bad request when the message format is incorrect"
// @Failure 500 {object} ErrorResponse "Internal server error if the publishing fails"
// @Router /publish/{topic} [post]
func (h *PublishHandler) Handle(w http.ResponseWriter, r *http.Request) {
    start := time.Now()

    // Start tracing span
    span, ctx := tracing.StartSpanFromContext(r.Context(), "PublishHandler.Handle", "")
    defer span.Finish()

    defer func() {
        metrics.HTTPRequestDuration.WithLabelValues("publish").Observe(time.Since(start).Seconds())
    }()

    vars := mux.Vars(r)
    topic := vars["topic"]

    h.logger.Infow("Received publish request", "topic", topic)

    if topic == "" {
        h.logger.Error("Missing topic path parameter")
        span.SetTag("error", true)
        span.LogKV("event", "missing_topic", "error", "Missing topic path parameter")
        respondWithError(w, r, http.StatusBadRequest, "Missing topic path parameter")
        metrics.HTTPRequestErrors.WithLabelValues("publish", "400").Inc()
        return
    }

    var message models.MessagePayload
    contentType := r.Header.Get("Content-Type")

    // Read and log the raw request body
    body, err := ioutil.ReadAll(r.Body)
    if err != nil {
        h.logger.Errorw("Failed to read request body", "error", err)
        respondWithError(w, r, http.StatusInternalServerError, "Failed to read request body")
        return
    }
    h.logger.Infow("Received request body", "body", string(body))

    // Reset the body for further reading
    r.Body = ioutil.NopCloser(bytes.NewBuffer(body))

    span.LogKV("event", "decoding_message", "contentType", contentType)
    err = json.Unmarshal(body, &message)
    if err != nil {
        h.logger.Errorw("Failed to decode message", "error", err, "content_type", contentType)
        span.SetTag("error", true)
        span.LogKV("event", "decoding_failed", "error", err)
        respondWithError(w, r, http.StatusBadRequest, "Invalid message format")
        metrics.HTTPRequestErrors.WithLabelValues("publish", "400").Inc()
        return
    }

    // Publish message
    span.LogKV("event", "publishing_message", "topic", topic)
    err = h.service.PublishMessage(ctx, topic, &message)
    if err != nil {
        h.logger.Errorw("Failed to publish message", "error", err, "topic", topic)
        span.SetTag("error", true)
        span.LogKV("event", "publish_failed", "error", err)
        respondWithError(w, r, http.StatusInternalServerError, "Failed to publish message")
        metrics.HTTPRequestErrors.WithLabelValues("publish", "500").Inc()
        return
    }

    span.LogKV("event", "message_published", "topic", topic)
    metrics.HTTPRequestsTotal.WithLabelValues("publish").Inc()
    h.logger.Infow("Message published successfully", "topic", topic, "message_id", message.Header.ID)

    respondWithSuccess(w, r, http.StatusAccepted, "Message published")
}

// respondWithSuccess writes a success message to the response.
func respondWithSuccess(w http.ResponseWriter, r *http.Request, statusCode int, message string) {
    response := SuccessResponse{
        Message: message,
    }

    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(statusCode)
    if err := json.NewEncoder(w).Encode(response); err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
    }
}
