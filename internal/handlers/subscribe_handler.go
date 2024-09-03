package handlers

import (

    "net/http"
    "time"

    "github.com/aanthord/pubsub-amqp/internal/amqp"
    "github.com/aanthord/pubsub-amqp/internal/metrics"
    "github.com/aanthord/pubsub-amqp/internal/tracing"
    "github.com/gorilla/mux"
    "go.uber.org/zap"
)

// SubscribeResponse represents a subscription response
type SubscribeResponse struct {
    Topic   string                 `json:"topic"`
    Message map[string]interface{} `json:"message"`
}

// SubscribeHandler handles subscribing to a topic.
type SubscribeHandler struct {
    service amqp.AMQPService
    logger  *zap.SugaredLogger
}

func NewSubscribeHandler(service amqp.AMQPService, logger *zap.SugaredLogger) *SubscribeHandler {
    return &SubscribeHandler{service: service, logger: logger}
}

// @Summary Subscribe to a topic
// @Description Subscribes to the specified AMQP topic and returns the next message
// @Tags messages
// @Accept json
// @Produce json
// @Param topic path string true "Topic to subscribe to"
// @Success 200 {object} SubscribeResponse "Successfully retrieved message"
// @Failure 204 "No message available at the moment"
// @Failure 400 {object} ErrorResponse "Bad request if topic is not provided"
// @Failure 500 {object} ErrorResponse "Internal server error if subscription fails"
// @Router /subscribe/{topic} [get]
func (h *SubscribeHandler) Handle(w http.ResponseWriter, r *http.Request) {
    start := time.Now()

    // Start tracing span
    span, ctx := tracing.StartSpanFromContext(r.Context(), "SubscribeHandler.Handle", "")
    defer span.Finish()

    defer func() {
        metrics.HTTPRequestDuration.WithLabelValues("subscribe").Observe(time.Since(start).Seconds())
    }()

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

    message, err := h.service.ConsumeMessage(ctx, topic)
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

    h.logger.Infow("Message received", "topic", topic, "message_id", message.Header.ID)

    // Start span for processing the received message
    processSpan, _ := tracing.StartSpanFromContext(ctx, "ProcessMessage", message.Header.TraceID)
    defer processSpan.Finish()

    var response SubscribeResponse
    response.Topic = topic
    response.Message = message.Content.Data

    h.logger.Infow("Message sent to client", "topic", topic)
    metrics.HTTPRequestsTotal.WithLabelValues("subscribe").Inc()

    respondWithJSON(w, http.StatusOK, response)
    h.logger.Infow("Subscribe request processed", "topic", topic)
}
