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

// Handle godoc
// @Summary Publish a message to a topic
// @Description Publishes a message to the specified AMQP topic
// @Tags messages
// @Accept json
// @Produce json
// @Param message body PublishRequest true "Message to publish"
// @Success 200 {object} PublishResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /publish [post]
func (h *PublishHandler) Handle(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	defer func() {
		metrics.HTTPRequestDuration.WithLabelValues("publish").Observe(time.Since(start).Seconds())
	}()

	var request PublishRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		h.logger.Errorw("Invalid request payload", "error", err)
		respondWithError(w, http.StatusBadRequest, "Invalid request payload")
		metrics.HTTPRequestErrors.WithLabelValues("publish", "400").Inc()
		return
	}

	if err := h.service.PublishMessage(r.Context(), request.Topic, []byte(request.Message)); err != nil {
		h.logger.Errorw("Failed to publish message", "error", err, "topic", request.Topic)
		respondWithError(w, http.StatusInternalServerError, "Failed to publish message")
		metrics.HTTPRequestErrors.WithLabelValues("publish", "500").Inc()
		return
	}

	respondWithJSON(w, http.StatusOK, PublishResponse{Status: "Message published"})
	h.logger.Infow("Message published successfully", "topic", request.Topic)
	metrics.HTTPRequestsTotal.WithLabelValues("publish").Inc()
}

type PublishRequest struct {
	Topic   string `json:"topic"`
	Message string `json:"message"`
}

type PublishResponse struct {
	Status string `json:"status"`
}
