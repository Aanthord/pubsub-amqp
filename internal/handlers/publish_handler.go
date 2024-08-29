package handlers

import (
	"encoding/json"
	"encoding/xml"
	"net/http"
	"strings"
	"time"

	"github.com/aanthord/pubsub-amqp/internal/amqp"
	"github.com/aanthord/pubsub-amqp/internal/metrics"
	"github.com/aanthord/pubsub-amqp/internal/models"
	"github.com/aanthord/pubsub-amqp/internal/tracing"
	"github.com/gorilla/mux"
	"go.uber.org/zap"
)

// SuccessResponse represents a generic success response
type SuccessResponse struct {
	Message string `json:"message" xml:"message"`
}

type PublishHandler struct {
	service amqp.AMQPService
	logger  *zap.SugaredLogger
}

func NewPublishHandler(service amqp.AMQPService, logger *zap.SugaredLogger) *PublishHandler {
	return &PublishHandler{service: service, logger: logger}
}

// Handle godoc
// @Summary Publish a message to a topic
// @Description Publishes a message to the specified AMQP topic
// @Tags messages
// @Accept application/json, application/xml
// @Produce application/json, application/xml
// @Param topic path string true "Topic to publish to"
// @Param message body models.MessagePayload true "Message payload"
// @Success 202 {object} SuccessResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /publish/{topic} [post]
func (h *PublishHandler) Handle(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	// Start the main span for the handler using your tracing package
	span, ctx := tracing.StartSpanFromContext(r.Context(), "PublishHandler.Handle", "")
	defer span.Finish()

	defer func() {
		metrics.HTTPRequestDuration.WithLabelValues("publish").Observe(time.Since(start).Seconds())
	}()

	// Extract the topic from the path
	span.LogKV("event", "extracting_topic")
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

	// Determine the request format (JSON or XML) based on the Content-Type header
	contentType := r.Header.Get("Content-Type")

	var message models.MessagePayload
	var err error

	if strings.Contains(contentType, "application/xml") {
		err = xml.NewDecoder(r.Body).Decode(&message)
	} else {
		err = json.NewDecoder(r.Body).Decode(&message)
	}

	if err != nil {
		h.logger.Errorw("Failed to decode message", "error", err, "content_type", contentType)
		span.SetTag("error", true)
		span.LogKV("event", "decode_error", "error", err.Error())
		respondWithError(w, r, http.StatusBadRequest, "Invalid message format")
		metrics.HTTPRequestErrors.WithLabelValues("publish", "400").Inc()
		return
	}

	// Log the message content
	h.logger.Debugw("Message content", "content", message)

	// Publish the message
	span.SetTag("topic", topic)
	publishSpan, publishCtx := tracing.StartSpanFromContext(ctx, "AMQP.PublishMessage", message.TraceID)
	defer publishSpan.Finish()

	err = h.service.PublishMessage(publishCtx, topic, &message)
	if err != nil {
		h.logger.Errorw("Failed to publish message", "error", err, "topic", topic)
		publishSpan.SetTag("error", true)
		publishSpan.LogKV("event", "publish_error", "error", err.Error())
		respondWithError(w, r, http.StatusInternalServerError, "Failed to publish message")
		metrics.HTTPRequestErrors.WithLabelValues("publish", "500").Inc()
		return
	}

	metrics.HTTPRequestsTotal.WithLabelValues("publish").Inc()
	h.logger.Infow("Message published successfully", "topic", topic, "message_id", message.ID)

	// Respond to the client
	respondWithSuccess(w, r, http.StatusAccepted, "Message published")
}

// respondWithSuccess writes a success message to the response
func respondWithSuccess(w http.ResponseWriter, r *http.Request, statusCode int, message string) {
	response := SuccessResponse{
		Message: message,
	}

	acceptHeader := r.Header.Get("Accept")

	if strings.Contains(acceptHeader, "application/xml") {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(statusCode)
		if err := xml.NewEncoder(w).Encode(response); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	} else {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		if err := json.NewEncoder(w).Encode(response); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}
