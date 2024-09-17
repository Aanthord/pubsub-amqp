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

// ValidationError is a custom struct to capture field-specific validation errors
type ValidationError struct {
    Field   string `json:"field"`
    Message string `json:"message"`
}

// LintingErrorResponse is a detailed error response for validation and linting issues
type LintingErrorResponse struct {
    Error       string            `json:"error"`
    Validations []ValidationError `json:"validations,omitempty"`
}

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
// @Failure 400 {object} LintingErrorResponse "Bad request when the message format is incorrect"
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

    // Read the raw request body
    body, err := ioutil.ReadAll(r.Body)
    if err != nil {
        h.logger.Errorw("Failed to read request body", "error", err)
        respondWithError(w, r, http.StatusInternalServerError, "Failed to read request body")
        return
    }

    // Log the raw request body
    h.logger.Infow("Received request body", "body", string(body))

    // Reset the body for further reading
    r.Body = ioutil.NopCloser(bytes.NewBuffer(body))

    span.LogKV("event", "decoding_message", "contentType", contentType)
    err = json.Unmarshal(body, &message)
    if err != nil {
        h.logger.Errorw("Failed to decode message", "error", err, "content_type", contentType)
        span.SetTag("error", true)
        span.LogKV("event", "decoding_failed", "error", err)

        // Return a detailed error response for JSON unmarshalling errors
        respondWithError(w, r, http.StatusBadRequest, "Invalid message format: "+err.Error())
        metrics.HTTPRequestErrors.WithLabelValues("publish", "400").Inc()
        return
    }

    // Validate the message payload
    validationErrors := validateMessagePayload(message)
    if len(validationErrors) > 0 {
        h.logger.Errorw("Message validation failed", "validation_errors", validationErrors)
        span.SetTag("error", true)
        span.LogKV("event", "validation_failed", "errors", validationErrors)

        respondWithLintingError(w, r, http.StatusBadRequest, validationErrors)
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

// validateMessagePayload performs basic validation on the MessagePayload.
func validateMessagePayload(payload models.MessagePayload) []ValidationError {
    var validationErrors []ValidationError

    if payload.Header.ID == "" {
        validationErrors = append(validationErrors, ValidationError{
            Field:   "Header.ID",
            Message: "Message ID is required",
        })
    }

    if payload.Header.Sender == "" {
        validationErrors = append(validationErrors, ValidationError{
            Field:   "Header.Sender",
            Message: "Sender is required",
        })
    }

    if payload.Header.Timestamp == "" {
        validationErrors = append(validationErrors, ValidationError{
            Field:   "Header.Timestamp",
            Message: "Timestamp is required",
        })
    }

    if payload.Header.TraceID == "" {
        validationErrors = append(validationErrors, ValidationError{
            Field:   "Header.TraceID",
            Message: "TraceID is required",
        })
    }

    // Add more checks as needed for other fields...

    return validationErrors
}

// respondWithLintingError returns a structured validation error response.
func respondWithLintingError(w http.ResponseWriter, r *http.Request, code int, validationErrors []ValidationError) {
    response := LintingErrorResponse{
        Error:       "Validation errors in the message payload",
        Validations: validationErrors,
    }
    respondWithJSON(w, code, response)
}
