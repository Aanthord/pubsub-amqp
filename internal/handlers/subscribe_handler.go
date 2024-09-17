package handlers

import (
    "context"
    "encoding/json"
    "net/http"
    "time"

    "github.com/aanthord/pubsub-amqp/internal/amqp"
    "github.com/aanthord/pubsub-amqp/internal/metrics"
    "github.com/aanthord/pubsub-amqp/internal/tracing"
    "github.com/clbanning/mxj/v2" // mxj library for handling XML and JSON
    "github.com/gorilla/mux"
    "go.uber.org/zap"
)

// SubscribeResponse represents a subscription response
type SubscribeResponse struct {
    Topic   string                 `json:"topic" xml:"topic"`
    Message map[string]interface{} `json:"message" xml:"message"`
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
// @Description Subscribes to the specified AMQP topic and returns the next message or a 204 if no message is available within the timeout period
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

    // Set up a context with a timeout for long polling (e.g., 30 seconds)
    timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
    defer cancel()

    message, err := h.service.ConsumeMessage(timeoutCtx, topic)
    if err != nil {
        if timeoutCtx.Err() == context.DeadlineExceeded {
            h.logger.Warn("No message available in the queue", "topic", topic)
            span.LogKV("event", "no_message_available", "topic", topic)
            w.WriteHeader(http.StatusNoContent) // 204 No Content
            metrics.HTTPRequestErrors.WithLabelValues("subscribe", "204").Inc()
            return
        }
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
        w.WriteHeader(http.StatusNoContent) // 204 No Content
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

    // Determine whether to respond with XML or JSON based on the Accept header
    acceptHeader := r.Header.Get("Accept")
    if acceptHeader == "application/xml" {
        respondWithXML(w, http.StatusOK, response)
    } else {
        respondWithJSON(w, http.StatusOK, response)
    }

    h.logger.Infow("Subscribe request processed", "topic", topic)
}


