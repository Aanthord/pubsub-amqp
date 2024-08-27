package uuid

import (
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type UUIDService interface {
	GenerateUUID() string
}

type uuidService struct {
	logger *zap.SugaredLogger
}

func NewUUIDService() UUIDService {
	logger, _ := zap.NewProduction()
	sugar := logger.Sugar()

	return &uuidService{logger: sugar}
}

func (s *uuidService) GenerateUUID() string {
	uuid := uuid.New().String()
	s.logger.Infow("Generated UUID", "uuid", uuid)
	return uuid
}
