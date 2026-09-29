package repo

import (
	"context"

	"piguard/go-backend/internal/apperr"
	"piguard/go-backend/internal/model"

	"gorm.io/gorm"
)

// EventRepository 读写 events 表。告警查询尚未实现。
type EventRepository interface {
	List(ctx context.Context, deviceID string) ([]model.Event, error)
}

type eventRepo struct {
	db *gorm.DB
}

func NewEventRepository(db *gorm.DB) EventRepository {
	return &eventRepo{db: db}
}

func (r *eventRepo) List(context.Context, string) ([]model.Event, error) {
	_ = r.db
	return nil, apperr.ErrNotImplemented
}
