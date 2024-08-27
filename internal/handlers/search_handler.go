package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/aanthord/pubsub-amqp/internal/metrics"
	"github.com/aanthord/pubsub-amqp/internal/search"
	"github.com/neo4j/neo4j-go-driver/v4/neo4j"
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
// @Param q query string true "Search query"
// @Success 200 {array} map[string]interface{}
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /search [get]
func (h *SearchHandler) Handle(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	defer func() {
		metrics.HTTPRequestDuration.WithLabelValues("search").Observe(time.Since(start).Seconds())
	}()

	query := r.URL.Query().Get("q")
	if query == "" {
		h.logger.Error("Missing search query parameter")
		respondWithError(w, http.StatusBadRequest, "Missing search query parameter")
		metrics.HTTPRequestErrors.WithLabelValues("search", "400").Inc()
		return
	}

	results, err := h.service.Search(r.Context(), query)
	if err != nil {
		h.logger.Errorw("Failed to perform search", "error", err, "query", query)
		respondWithError(w, http.StatusInternalServerError, "Failed to perform search")
		metrics.HTTPRequestErrors.WithLabelValues("search", "500").Inc()
		return
	}

	respondWithJSON(w, http.StatusOK, resultsToMap(results))
	
	h.logger.Infow("Search performed successfully", "query", query, "results", len(results))
	metrics.HTTPRequestsTotal.WithLabelValues("search").Inc()
}

func resultsToMap(records []neo4j.Record) []map[string]interface{} {
	var result []map[string]interface{}
	for _, record := range records {
		m := make(map[string]interface{})
		for i := 0; i < record.Length(); i++ {
			m[record.Keys()[i]] = record.Values()[i]
		}
		result = append(result, m)
	}
	return result
}
