package handlers

import (
    "bytes"
    "encoding/json"
    "encoding/xml"
    "io/ioutil"
    "net/http"
    "time"

    "github.com/aanthord/pubsub-amqp/internal/amqp"
    "github.com/aanthord/pubsub-amqp/internal/metrics"
    "github.com/aanthord/pubsub-amqp/internal/models"
    "github.com/aanthord/pubsub-amqp/internal/tracing"
    "github.com/clbanning/mxj/v2"
    "github.com/gorilla/mux"
    "go.uber.org/zap"
)

// ValidationError is a custom struct to capture field-specific validation errors
type ValidationError struct {
    Field   string `json:"field" xml:"field"`
    Message string `json:"message" xml:"message"`
}

// LintingErrorResponse is a detailed error response for validation and linting issues
type LintingErrorResponse struct {
    Error       string            `json:"error" xml:"error"`
    Validations []ValidationError `json:"validations,omitempty" xml:"validations,omitempty"`
}

// SuccessResponse represents a generic success response
type SuccessResponse struct {
    Message string `json:"message" xml:"message"`
}

// PublishHandler handles publishing messages to a topic.
type PublishHandler struct {
    service amqp.AMQPService
    logger  *zap.SugaredLogger
}

func NewPublishHandler(service amqp.AMQPService, logger *zap.SugaredLogger) *PublishHandler {
    return &PublishHandler{service: service, logger: logger}
}

// @Summary Publish a message to a topic (Supports JSON and XML)
// @Description Publishes a message to the specified AMQP topic. This documentation covers JSON payload, but the handler also supports XML.
// @Tags messages
// @Accept json
// @Produce json
// @Param topic path string true "Topic to publish to"
// @Param message body models.MessagePayload true "Message payload in JSON"
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

    // Handle based on content type (XML or JSON)
    if contentType == "application/xml" {
        mv, err := mxj.NewMapXml(body)
        if err != nil {
            h.logger.Errorw("Failed to parse XML", "error", err)
            span.SetTag("error", true)
            respondWithError(w, r, http.StatusBadRequest, "Invalid XML format")
            metrics.HTTPRequestErrors.WithLabelValues("publish", "400").Inc()
            return
        }
        message = parseMessagePayloadFromMap(mv)
    } else if contentType == "application/json" {
        err = json.Unmarshal(body, &message)
        if err != nil {
            h.logger.Errorw("Failed to decode message", "error", err, "content_type", contentType)
            span.SetTag("error", true)
            respondWithError(w, r, http.StatusBadRequest, "Invalid message format: "+err.Error())
            metrics.HTTPRequestErrors.WithLabelValues("publish", "400").Inc()
            return
        }
    } else {
        h.logger.Errorw("Unsupported content type", "content_type", contentType)
        respondWithError(w, r, http.StatusUnsupportedMediaType, "Unsupported content type")
        return
    }

    // Validate the message payload
    validationErrors := validateMessagePayload(message)
    if len(validationErrors) > 0 {
        h.logger.Errorw("Message validation failed", "validation_errors", validationErrors)
        span.SetTag("error", true)
        respondWithLintingError(w, r, http.StatusBadRequest, validationErrors)
        metrics.HTTPRequestErrors.WithLabelValues("publish", "400").Inc()
        return
    }

    // Publish the message
    err = h.service.PublishMessage(ctx, topic, &message)
    if err != nil {
        h.logger.Errorw("Failed to publish message", "error", err, "topic", topic)
        span.SetTag("error", true)
        respondWithError(w, r, http.StatusInternalServerError, "Failed to publish message")
        metrics.HTTPRequestErrors.WithLabelValues("publish", "500").Inc()
        return
    }

    h.logger.Infow("Message published successfully", "topic", topic, "message_id", message.Header.ID)
    metrics.HTTPRequestsTotal.WithLabelValues("publish").Inc()

    respondWithSuccess(w, r, http.StatusAccepted, "Message published")
}

// parseMessagePayloadFromMap converts an XML map to a MessagePayload
func parseMessagePayloadFromMap(mv mxj.Map) models.MessagePayload {
    var message models.MessagePayload
    header, _ := mv.ValueForPath("MessagePayload.Header")
    content, _ := mv.ValueForPath("MessagePayload.Content")

    if header != nil {
        jsonHeader, _ := json.Marshal(header)
        _ = json.Unmarshal(jsonHeader, &message.Header)
    }

    if content != nil {
        jsonContent, _ := json.Marshal(content)
        _ = json.Unmarshal(jsonContent, &message.Content)
    }

    return message
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

    return validationErrors
}

// respondWithLintingError returns a structured validation error response.
func respondWithLintingError(w http.ResponseWriter, r *http.Request, code int, validationErrors []ValidationError) {
    response := LintingErrorResponse{
        Error:       "Validation errors in the message payload",
        Validations: validationErrors,
    }
    respondWith(w, code, response, r)
}

// respondWithSuccess sends a success response.
func respondWithSuccess(w http.ResponseWriter, r *http.Request, code int, message string) {
    respondWith(w, code, SuccessResponse{Message: message}, r)
}

// respondWith encodes the response as JSON or XML depending on the request's Accept header
func respondWith(w http.ResponseWriter, code int, payload interface{}, r *http.Request) {
    acceptHeader := r.Header.Get("Accept")

    if acceptHeader == "application/xml" {
        response, _ := xml.Marshal(payload)
        w.Header().Set("Content-Type", "application/xml")
        w.WriteHeader(code)
        w.Write(response)
    } else {
        response, _ := json.Marshal(payload)
        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(code)
        w.Write(response)
    }
}
