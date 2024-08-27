package handlers

import (
	"net/http"
	"time"

	"github.com/aanthord/pubsub-amqp/internal/metrics"
	"github.com/aanthord/pubsub-amqp/internal/uuid"
	"go.uber.org/zap"
)

type UUIDHandler struct {
	service uuid.UUIDService
	logger  *zap.SugaredLogger
}

func NewUUIDHandler(service uuid.UUIDService) *UUIDHandler {
	logger, _ := zap.NewProduction()
	sugar := logger.Sugar()

	return &UUIDHandler{service: service, logger: sugar}
}

// Handle godoc
// @Summary Generate a UUID
// @Description Generates a new UUID
// @Tags uuid
// @Produce json
// @Success 200 {object} UUIDResponse
// @Router /uuid [get]
func (h *UUIDHandler) Handle(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	defer func() {
		metrics.HTTPRequestDuration.WithLabelValues("uuid").Observe(time.Since(start).Seconds())
	}()

	uuid := h.service.GenerateUUID()

	respondWithJSON(w, r, http.StatusOK, UUIDResponse{UUID: uuid})

	h.logger.Infow("UUID generated", "uuid", uuid)
	metrics.HTTPRequestsTotal.WithLabelValues("uuid").Inc()
}

type UUIDResponse struct {
	UUID string `json:"uuid"`
}
