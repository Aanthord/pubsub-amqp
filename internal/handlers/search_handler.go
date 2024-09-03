package handlers

import (
    "net/http"
    "time"

    "github.com/aanthord/pubsub-amqp/internal/metrics"
    "github.com/aanthord/pubsub-amqp/internal/models"
    "github.com/aanthord/pubsub-amqp/internal/search"
    "go.uber.org/zap"
)

type SearchHandler struct {
    service search.SearchService
    logger  *zap.SugaredLogger
}

func NewSearchHandler(service search.SearchService) *SearchHandler {
    logger, _ := zap.NewProduction()
    sugar := logger.Sugar()

    return &SearchHandler{service: service, logger: sugar}
}

// Handle godoc
// @Summary Search for nodes
// @Description Searches for nodes in the graph database
// @Tags search
// @Produce json
// @Param trace_id query string true "Trace ID"
// @Success 200 {array} map[string]interface{}
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /search [get]
func (h *SearchHandler) Handle(w http.ResponseWriter, r *http.Request) {
    start := time.Now()
    defer func() {
        metrics.HTTPRequestDuration.WithLabelValues("search").Observe(time.Since(start).Seconds())
    }()

    traceID := r.URL.Query().Get("trace_id")
    if traceID == "" {
        h.logger.Error("Missing search query parameter")
        respondWithError(w, r, http.StatusBadRequest, "Missing search query parameter")
        metrics.HTTPRequestErrors.WithLabelValues("search", "400").Inc()
        return
    }

    results, err := h.service.SearchByTraceID(r.Context(), traceID)
    if err != nil {
        h.logger.Errorw("Failed to perform search", "error", err, "trace_id", traceID)
        respondWithError(w, r, http.StatusInternalServerError, "Failed to perform search")
        metrics.HTTPRequestErrors.WithLabelValues("search", "500").Inc()
        return
    }

    respondWithJSON(w, http.StatusOK, resultsToMap(results))

    h.logger.Infow("Search performed successfully", "trace_id", traceID, "results", len(results))
    metrics.HTTPRequestsTotal.WithLabelValues("search").Inc()
}

func resultsToMap(messages []*models.MessagePayload) []map[string]interface{} {
    var result []map[string]interface{}
    for _, message := range messages {
        m := map[string]interface{}{
            "id":        message.Header.ID,
            "trace_id":  message.Header.TraceID,
            "sender":    message.Header.Sender,
            "timestamp": message.Header.Timestamp,
            "version":   message.Header.Version,
            "retries":   message.Header.Retries,
            "content":   message.Content.Data,
        }
        result = append(result, m)
    }
    return result
}
