package handlers

import (
	"encoding/json"
	"encoding/xml"
	"net/http"
	"strings"
	"time"

	"github.com/aanthord/pubsub-amqp/internal/amqp"
	"github.com/aanthord/pubsub-amqp/internal/metrics"
	"github.com/aanthord/pubsub-amqp/internal/tracing"
	"github.com/gorilla/mux"
	"go.uber.org/zap"
)

type SubscribeHandler struct {
	service amqp.AMQPService
	logger  *zap.SugaredLogger
}

type SubscribeResponse struct {
	Topic   string `json:"topic" xml:"topic"`
	Message string `json:"message" xml:"message"`
}

func NewSubscribeHandler(service amqp.AMQPService, logger *zap.SugaredLogger) *SubscribeHandler {
	return &SubscribeHandler{service: service, logger: logger}
}

// Handle godoc
// @Summary Subscribe to a topic
// @Description Subscribes to the specified AMQP topic and returns the next message
// @Produce application/json, application/xml
// @Tags messages
// @Param topic path string true "Topic to subscribe to"
// @Success 200 {object} SubscribeResponse
// @Failure 204 "No message available"
// @Failure 400 {object} ErrorResponse
// @Failure 408 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /subscribe/{topic} [get]
func (h *SubscribeHandler) Handle(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	// Start the main span for the handler using your tracing package
	span, ctx := tracing.StartSpanFromContext(r.Context(), "SubscribeHandler.Handle", "")
	defer span.Finish()

	defer func() {
		metrics.HTTPRequestDuration.WithLabelValues("subscribe").Observe(time.Since(start).Seconds())
	}()

	// Extract the topic from the path
	span.LogKV("event", "extracting_topic")
	vars := mux.Vars(r)
	topic := vars["topic"]

	h.logger.Infow("Received subscribe request", "topic", topic)

	if topic == "" {
		h.logger.Error("Missing topic path parameter")
		span.SetTag("error", true)
		span.LogKV("event", "missing_topic", "error", "Missing topic path parameter")
		respondWithError(w, r, http.StatusBadRequest, "Missing topic path parameter")
		metrics.HTTPRequestErrors.WithLabelValues("subscribe", "400").Inc()
		return
	}

	// Start span for AMQP message consumption
	consumeSpan, consumeCtx := tracing.StartSpanFromContext(ctx, "AMQP.ConsumeMessage", "")
	message, err := h.service.ConsumeMessage(consumeCtx, topic)
	consumeSpan.Finish()

	if err != nil {
		h.logger.Errorw("Failed to subscribe to topic", "error", err, "topic", topic)
		span.SetTag("error", true)
		span.LogKV("event", "amqp_consume_error", "error", err.Error())
		respondWithError(w, r, http.StatusInternalServerError, "Failed to subscribe to topic")
		metrics.HTTPRequestErrors.WithLabelValues("subscribe", "500").Inc()
		return
	}

	if message == nil {
		h.logger.Warn("No message available in the queue", "topic", topic)
		span.LogKV("event", "no_message_available", "topic", topic)
		w.WriteHeader(http.StatusNoContent)
		metrics.HTTPRequestErrors.WithLabelValues("subscribe", "204").Inc()
		return
	}

	h.logger.Infow("Message received", "topic", topic, "message_id", message.ID)

	// Start span for processing the received message
	processSpan, _ := tracing.StartSpanFromContext(ctx, "ProcessMessage", message.TraceID)
	defer processSpan.Finish()

	// Determine the response format (JSON or XML) based on the Accept header
	acceptHeader := r.Header.Get("Accept")

	var response SubscribeResponse
	response.Topic = topic

	if strings.Contains(acceptHeader, "application/xml") {
		contentXML, err := xml.Marshal(message.Content)
		if err != nil {
			h.logger.Errorw("Failed to marshal message content to XML", "error", err)
			respondWithError(w, r, http.StatusInternalServerError, "Failed to process message content")
			metrics.HTTPRequestErrors.WithLabelValues("subscribe", "500").Inc()
			return
		}
		response.Message = string(contentXML)
		respondWithXML(w, r, http.StatusOK, response)
	} else {
		contentJSON, err := json.Marshal(message.Content)
		if err != nil {
			h.logger.Errorw("Failed to marshal message content to JSON", "error", err)
			respondWithError(w, r, http.StatusInternalServerError, "Failed to process message content")
			metrics.HTTPRequestErrors.WithLabelValues("subscribe", "500").Inc()
			return
		}
		response.Message = string(contentJSON)
		respondWithJSON(w, r, http.StatusOK, response)
	}

	h.logger.Infow("Message sent to client", "topic", topic)

	metrics.HTTPRequestsTotal.WithLabelValues("subscribe").Inc()
	h.logger.Infow("Subscribe request processed", "topic", topic)
}
