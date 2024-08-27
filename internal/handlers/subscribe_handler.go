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

func (h *SubscribeHandler) Handle(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	defer func() {
		metrics.HTTPRequestDuration.WithLabelValues("subscribe").Observe(time.Since(start).Seconds())
	}()

	topic := r.URL.Query().Get("topic")
	if topic == "" {
		h.logger.Error("Missing topic query parameter")
		http.Error(w, "Missing topic query parameter", http.StatusBadRequest)
		metrics.HTTPRequestErrors.WithLabelValues("subscribe", "400").Inc()
		return
	}

	msgChan, err := h.service.SubscribeToTopic(r.Context(), topic)
	if err != nil {
		h.logger.Errorw("Failed to subscribe to topic", "error", err, "topic", topic)
		http.Error(w, "Failed to subscribe to topic", http.StatusInternalServerError)
		metrics.HTTPRequestErrors.WithLabelValues("subscribe", "500").Inc()
		return
	}

	select {
	case message := <-msgChan:
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"topic":   topic,
			"message": string(message),
		})
		h.logger.Infow("Message received from subscription", "topic", topic)
	case <-r.Context().Done():
		h.logger.Warn("Client disconnected before receiving message")
		http.Error(w, "Request cancelled", http.StatusRequestTimeout)
		metrics.HTTPRequestErrors.WithLabelValues("subscribe", "408").Inc()
		return
	}

	metrics.HTTPRequestsTotal.WithLabelValues("subscribe").Inc()
}
