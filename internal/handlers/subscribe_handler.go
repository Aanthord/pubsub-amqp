package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/aanthord/pubsub-amqp/internal/amqp"
	"github.com/aanthord/pubsub-amqp/internal/metrics"
	"go.uber.org/zap"
)

type SubscribeHandler struct {
	service amqp.AMQPService
	logger  *zap.SugaredLogger
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
// @Param topic query string true "Topic to subscribe to"
// @Success 200 {object} SubscribeResponse
// @Failure 400 {object} ErrorResponse
// @Failure 408 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /subscribe [get]
func (h *SubscribeHandler) Handle(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	defer func() {
		metrics.HTTPRequestDuration.WithLabelValues("subscribe").Observe(time.Since(start).Seconds())
	}()

	topic := r.URL.Query().Get("topic")
	if topic == "" {
		h.logger.Error("Missing topic query parameter")
		respondWithError(w, r, http.StatusBadRequest, "Missing topic query parameter")
		metrics.HTTPRequestErrors.WithLabelValues("subscribe", "400").Inc()
		return
	}

	msgChan, err := h.service.ConsumeMessages(r.Context(), topic)
	if err != nil {
		h.logger.Errorw("Failed to subscribe to topic", "error", err, "topic", topic)
		respondWithError(w, r, http.StatusInternalServerError, "Failed to subscribe to topic")
		metrics.HTTPRequestErrors.WithLabelValues("subscribe", "500").Inc()
		return
	}

	select {
	case message := <-msgChan:
		// Marshal the content to a JSON string
		contentJSON, err := json.Marshal(message.Content)
		if err != nil {
			h.logger.Errorw("Failed to marshal message content", "error", err)
			respondWithError(w, r, http.StatusInternalServerError, "Failed to process message content")
			metrics.HTTPRequestErrors.WithLabelValues("subscribe", "500").Inc()
			return
		}

		respondWithJSON(w, r, http.StatusOK, SubscribeResponse{
			Topic:   topic,
			Message: string(contentJSON),
		})
		h.logger.Infow("Message received from subscription", "topic", topic)
	case <-r.Context().Done():
		h.logger.Warn("Client disconnected before receiving message")
		respondWithError(w, r, http.StatusRequestTimeout, "Request cancelled")
		metrics.HTTPRequestErrors.WithLabelValues("subscribe", "408").Inc()
		return
	}

	metrics.HTTPRequestsTotal.WithLabelValues("subscribe").Inc()
}

type SubscribeResponse struct {
	Topic   string `json:"topic"`
	Message string `json:"message"`
}

