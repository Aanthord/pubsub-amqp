package handlers

import (
	"encoding/json"
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

func (h *UUIDHandler) Handle(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	defer func() {
		metrics.HTTPRequestDuration.WithLabelValues("uuid").Observe(time.Since(start).Seconds())
	}()

	uuid := h.service.GenerateUUID()
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"uuid": uuid})
	
	h.logger.Infow("UUID generated", "uuid", uuid)
	metrics.HTTPRequestsTotal.WithLabelValues("uuid").Inc()
}
