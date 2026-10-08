package repo

import (
	"context"
	"time"

	"piguard/go-backend/internal/model"

	"gorm.io/gorm"
)

type TelemetryQuery struct {
	Start *time.Time
	End   *time.Time
	Limit int
}

// TelemetryRepository 读写每设备 1Hz 的历史快照。
type TelemetryRepository interface {
	Insert(context.Context, *model.Telemetry) error
	List(context.Context, string, TelemetryQuery) ([]model.Telemetry, error)
	Latest(context.Context, string) (*model.Telemetry, error)
}

type telemetryRepo struct {
	db *gorm.DB
}

func NewTelemetryRepository(db *gorm.DB) TelemetryRepository {
	return &telemetryRepo{db: db}
}

func (r *telemetryRepo) Insert(ctx context.Context, t *model.Telemetry) error {
	return r.db.WithContext(ctx).Create(t).Error
}

func (r *telemetryRepo) Latest(ctx context.Context, id string) (*model.Telemetry, error) {
	var row model.Telemetry
	result := r.db.WithContext(ctx).Where("device_id = ?", id).Order("timestamp DESC, seq DESC, id DESC").Limit(1).Find(&row)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return &row, nil
}

func (r *telemetryRepo) List(ctx context.Context, deviceID string, query TelemetryQuery) ([]model.Telemetry, error) {
	q := r.db.WithContext(ctx).Where("device_id = ?", deviceID)
	if query.Start != nil {
		q = q.Where("timestamp >= ?", *query.Start)
	}
	if query.End != nil {
		q = q.Where("timestamp <= ?", *query.End)
	}
	items := []model.Telemetry{}
	err := q.Order("timestamp DESC, seq DESC, id DESC").Limit(query.Limit).Find(&items).Error
	for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
		items[i], items[j] = items[j], items[i]
	}
	return items, err
}
