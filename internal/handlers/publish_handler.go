package handlers

import (
    "net/http"
    "time"

    "github.com/aanthord/pubsub-amqp/internal/amqp"
    "github.com/aanthord/pubsub-amqp/internal/httputil"
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

type PublishRequest struct {
    Sender  string                 `json:"sender" xml:"sender"`
    Content map[string]interface{} `json:"content" xml:"content"`
}

type PublishResponse struct {
    Status string `json:"status" xml:"status"`
}

// Handle godoc
// @Summary Publish a message
// @Description Publishes a message to the specified topic
// @Tags messages
// @Accept json
// @Accept xml
// @Produce json
// @Produce xml
// @Param topic path string true "Topic to publish to"
// @Param message body PublishRequest true "Message to publish"
// @Success 200 {object} PublishResponse
// @Failure 400 {object} httputil.ErrorResponse
// @Failure 500 {object} httputil.ErrorResponse
// @Router /publish/{topic} [post]
func (h *PublishHandler) Handle(w http.ResponseWriter, r *http.Request) {
    start := time.Now()
    defer func() {
        metrics.HTTPRequestDuration.WithLabelValues("publish").Observe(time.Since(start).Seconds())
    }()

    vars := mux.Vars(r)
    topic := vars["topic"]

    var request PublishRequest
    if err := httputil.Decode(r, r.Body, &request); err != nil {
        h.logger.Errorw("Failed to decode request", "error", err)
        httputil.RespondWithError(w, r, http.StatusBadRequest, "Invalid request payload")
        metrics.HTTPRequestErrors.WithLabelValues("publish", "400").Inc()
        return
    }

    messagePayload := models.NewMessagePayload(request.Sender, request.Content, "")

    if err := h.amqpService.PublishMessage(r.Context(), topic, messagePayload); err != nil {
        h.logger.Errorw("Failed to publish message", "error", err, "topic", topic)
        httputil.RespondWithError(w, r, http.StatusInternalServerError, "Failed to publish message")
        metrics.HTTPRequestErrors.WithLabelValues("publish", "500").Inc()
        return
    }

    response := PublishResponse{Status: "Message published"}
    if err := httputil.RespondWithData(w, r, http.StatusOK, response); err != nil {
        h.logger.Errorw("Failed to encode response", "error", err)
        httputil.RespondWithError(w, r, http.StatusInternalServerError, "Failed to encode response")
        metrics.HTTPRequestErrors.WithLabelValues("publish", "500").Inc()
        return
    }

    h.logger.Infow("Message published successfully", "topic", topic)
    metrics.HTTPRequestsTotal.WithLabelValues("publish").Inc()
}