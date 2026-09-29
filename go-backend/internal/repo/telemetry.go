package repo

import (
	"context"

	"piguard/go-backend/internal/apperr"
	"piguard/go-backend/internal/model"

	"gorm.io/gorm"
)

// TelemetryRepository 读写 telemetry 表。历史查询尚未实现。
type TelemetryRepository interface {
	List(ctx context.Context, deviceID string) ([]model.Telemetry, error)
}

type telemetryRepo struct {
	db *gorm.DB
}

func NewTelemetryRepository(db *gorm.DB) TelemetryRepository {
	return &telemetryRepo{db: db}
}

func (r *telemetryRepo) List(context.Context, string) ([]model.Telemetry, error) {
	// 保留数据库句柄，下一步在这里按时间范围查询。
	_ = r.db
	return nil, apperr.ErrNotImplemented
}
