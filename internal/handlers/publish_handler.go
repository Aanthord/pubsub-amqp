package handlers

import (
    "encoding/json"
    "net/http"
    "time"

    "github.com/aanthord/pubsub-amqp/internal/amqp"
    "github.com/aanthord/pubsub-amqp/internal/metrics"
    "github.com/aanthord/pubsub-amqp/internal/models"
    "github.com/gorilla/mux"
    "go.uber.org/zap"
)

type PublishHandler struct {
    amqpService amqp.AMQPService
    logger      *zap.SugaredLogger
}

func NewPublishHandler(amqpService amqp.AMQPService, logger *zap.SugaredLogger) *PublishHandler {
    return &PublishHandler{
        amqpService: amqpService,
        logger:      logger,
    }
}

func (h *PublishHandler) Handle(w http.ResponseWriter, r *http.Request) {
    start := time.Now()
    defer func() {
        metrics.HTTPRequestDuration.WithLabelValues("publish").Observe(time.Since(start).Seconds())
    }()

    vars := mux.Vars(r)
    topic := vars["topic"]

    var request PublishRequest
    if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
        h.logger.Errorw("Invalid request payload", "error", err)
        respondWithError(w, r, http.StatusBadRequest, "Invalid request payload")
        metrics.HTTPRequestErrors.WithLabelValues("publish", "400").Inc()
        return
    }

    messagePayload := models.NewMessagePayload(request.Sender, request.Content, "")

    if err := h.amqpService.PublishMessage(r.Context(), topic, messagePayload); err != nil {
        h.logger.Errorw("Failed to publish message", "error", err, "topic", topic)
        respondWithError(w, r, http.StatusInternalServerError, "Failed to publish message")
        metrics.HTTPRequestErrors.WithLabelValues("publish", "500").Inc()
        return
    }

    respondWithJSON(w, r, http.StatusOK, PublishResponse{Status: "Message published"})
    h.logger.Infow("Message published successfully", "topic", topic)
    metrics.HTTPRequestsTotal.WithLabelValues("publish").Inc()
}

type PublishRequest struct {
    Sender  string                 `json:"sender"`
    Content map[string]interface{} `json:"content"`
}

type PublishResponse struct {
    Status string `json:"status"`
}