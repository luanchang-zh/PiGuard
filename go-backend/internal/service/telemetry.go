package service

import (
	"context"

	"piguard/go-backend/internal/model"
	"piguard/go-backend/internal/repo"
)

// TelemetryService 提供历史遥测查询。入库和降频还没做。
type TelemetryService interface {
	List(ctx context.Context, deviceID string) ([]model.Telemetry, error)
}

type telemetryService struct {
	telemetry repo.TelemetryRepository
}

func NewTelemetryService(telemetry repo.TelemetryRepository) TelemetryService {
	return &telemetryService{telemetry: telemetry}
}

func (s *telemetryService) List(ctx context.Context, deviceID string) ([]model.Telemetry, error) {
	return s.telemetry.List(ctx, deviceID)
}
