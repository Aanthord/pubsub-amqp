package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/aanthord/pubsub-amqp/internal/amqp"
	"github.com/aanthord/pubsub-amqp/internal/metrics"
	"go.uber.org/zap"
)

type PublishHandler struct {
	service amqp.AMQPService
	logger  *zap.SugaredLogger
}

func NewPublishHandler(service amqp.AMQPService) *PublishHandler {
	logger, _ := zap.NewProduction()
	sugar := logger.Sugar()

	return &PublishHandler{service: service, logger: sugar}
}

func (h *PublishHandler) Handle(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	defer func() {
		metrics.HTTPRequestDuration.WithLabelValues("publish").Observe(time.Since(start).Seconds())
	}()

	var request struct {
		Topic   string `json:"topic"`
		Message string `json:"message"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		h.logger.Errorw("Invalid request payload", "error", err)
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		metrics.HTTPRequestErrors.WithLabelValues("publish", "400").Inc()
		return
	}

	if err := h.service.PublishMessage(r.Context(), request.Topic, []byte(request.Message)); err != nil {
		h.logger.Errorw("Failed to publish message", "error", err, "topic", request.Topic)
		http.Error(w, "Failed to publish message", http.StatusInternalServerError)
		metrics.HTTPRequestErrors.WithLabelValues("publish", "500").Inc()
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "Message published"})
	h.logger.Infow("Message published successfully", "topic", request.Topic)
	metrics.HTTPRequestsTotal.WithLabelValues("publish").Inc()
}
