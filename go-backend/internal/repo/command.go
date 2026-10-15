package repo

import (
	"context"
	"errors"
	"piguard/go-backend/internal/model"
	"time"

	"gorm.io/gorm"
)

// CommandRepository keeps transitions conditional so callbacks cannot regress terminal states.
type CommandRepository interface {
	Find(ctx context.Context, commandID string) (*model.Command, error)
	Insert(ctx context.Context, command *model.Command) error
	Expired(ctx context.Context, at time.Time) ([]model.Command, error)
	Transition(ctx context.Context, next *model.Command, from []string) (bool, error)
}

type commandRepo struct {
	db *gorm.DB
}

func NewCommandRepository(db *gorm.DB) CommandRepository {
	return &commandRepo{db: db}
}

func (r *commandRepo) Find(ctx context.Context, id string) (*model.Command, error) {
	var row model.Command
	err := r.db.WithContext(ctx).Where("command_id = ?", id).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}
func (r *commandRepo) Insert(ctx context.Context, row *model.Command) error {
	return r.db.WithContext(ctx).Create(row).Error
}
func (r *commandRepo) Expired(ctx context.Context, at time.Time) ([]model.Command, error) {
	rows := []model.Command{}
	err := r.db.WithContext(ctx).Where("status IN ? AND expires_at <= ?", []string{"pending", "sent"}, at).Order("expires_at, command_id").Find(&rows).Error
	return rows, err
}
func (r *commandRepo) Transition(ctx context.Context, next *model.Command, from []string) (bool, error) {
	result := r.db.WithContext(ctx).Model(&model.Command{}).Where("command_id = ? AND device_id = ? AND status IN ?", next.CommandID, next.DeviceID, from).Updates(map[string]any{
		"status": next.Status, "ack_at": next.AckAt, "executed_at": next.ExecutedAt, "result": next.Result, "error": next.Error,
	})
	return result.RowsAffected == 1, result.Error
}
