package repo

import (
	"context"
	"errors"
	"time"

	"piguard/go-backend/internal/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// EventQuery 是告警历史的筛选条件。Type 或 Level 为 nil 表示不按该字段过滤。
type EventQuery struct {
	Type  *string
	Level *string
	Start *time.Time
	End   *time.Time
	Limit int
}

// EventRepository 读写不可变的告警消息。
type EventRepository interface {
	Insert(context.Context, *model.Event) error
	Find(context.Context, string) (*model.Event, error)
	List(context.Context, string, EventQuery) ([]model.Event, error)
}

type eventRepo struct{ db *gorm.DB }

func NewEventRepository(db *gorm.DB) EventRepository { return &eventRepo{db: db} }

func (r *eventRepo) Insert(ctx context.Context, row *model.Event) error {
	result := r.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "event_id"}}, DoNothing: true}).Create(row)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrDuplicate
	}
	return nil
}

func (r *eventRepo) Find(ctx context.Context, eventID string) (*model.Event, error) {
	var row model.Event
	err := r.db.WithContext(ctx).Where("event_id = ?", eventID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *eventRepo) List(ctx context.Context, deviceID string, query EventQuery) ([]model.Event, error) {
	q := r.db.WithContext(ctx).Where("device_id = ?", deviceID)
	if query.Type != nil {
		q = q.Where("type = ?", *query.Type)
	}
	if query.Level != nil {
		q = q.Where("level = ?", *query.Level)
	}
	if query.Start != nil {
		q = q.Where("timestamp >= ?", *query.Start)
	}
	if query.End != nil {
		q = q.Where("timestamp <= ?", *query.End)
	}
	items := []model.Event{}
	err := q.Order("timestamp DESC, event_id DESC").Limit(query.Limit).Find(&items).Error
	return items, err
}

func DuplicateEvent(err error) bool {
	return errors.Is(err, ErrDuplicate)
}
