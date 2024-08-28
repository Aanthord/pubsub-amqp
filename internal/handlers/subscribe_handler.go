package handlers

import (
	"encoding/json"
	"net/http"
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
	Topic   string `json:"topic"`
	Message string `json:"message"`
}

func NewSubscribeHandler(service amqp.AMQPService) *SubscribeHandler {
	logger, _ := zap.NewProduction()
	sugar := logger.Sugar()

	return &SubscribeHandler{service: service, logger: sugar}
}

// Handle godoc
// @Summary Subscribe to a topic
// @Description Subscribes to the specified AMQP topic and returns the next message
// @Tags messages
// @Produce json
// @Param topic path string true "Topic to subscribe to"
// @Success 200 {object} SubscribeResponse
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
	consumeSpan, consumeCtx := tracing.StartSpanFromContext(ctx, "AMQP.ConsumeMessages", "")
	msgChan, err := h.service.ConsumeMessages(consumeCtx, topic)
	consumeSpan.Finish()

	if err != nil {
		h.logger.Errorw("Failed to subscribe to topic", "error", err, "topic", topic)
		span.SetTag("error", true)
		span.LogKV("event", "amqp_consume_error", "error", err.Error())
		respondWithError(w, r, http.StatusInternalServerError, "Failed to subscribe to topic")
		metrics.HTTPRequestErrors.WithLabelValues("subscribe", "500").Inc()
		return
	}

	// Wait for the message or context cancellation
	h.logger.Infow("Waiting for message", "topic", topic)
	select {
	case message := <-msgChan:
		h.logger.Infow("Message received", "topic", topic, "message_id", message.ID)

		// Start span for processing the received message
		processSpan, _ := tracing.StartSpanFromContext(ctx, "ProcessMessage", message.TraceID)
		defer processSpan.Finish()

		contentJSON, err := json.Marshal(message.Content)
		if err != nil {
			h.logger.Errorw("Failed to marshal message content", "error", err)
			processSpan.SetTag("error", true)
			processSpan.LogKV("event", "json_marshal_error", "error", err.Error())
			respondWithError(w, r, http.StatusInternalServerError, "Failed to process message content")
			metrics.HTTPRequestErrors.WithLabelValues("subscribe", "500").Inc()
			return
		}

		respondWithJSON(w, r, http.StatusOK, SubscribeResponse{
			Topic:   topic,
			Message: string(contentJSON),
		})
		h.logger.Infow("Message sent to client", "topic", topic)

	case <-r.Context().Done():
		h.logger.Warn("Client disconnected before receiving message", "topic", topic)
		span.LogKV("event", "client_disconnected")
		respondWithError(w, r, http.StatusRequestTimeout, "Request cancelled")
		metrics.HTTPRequestErrors.WithLabelValues("subscribe", "408").Inc()
	}

	metrics.HTTPRequestsTotal.WithLabelValues("subscribe").Inc()
	h.logger.Infow("Subscribe request processed", "topic", topic)
}
