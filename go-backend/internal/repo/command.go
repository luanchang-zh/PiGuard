package repo

import (
	"context"

	"piguard/go-backend/internal/apperr"
	"piguard/go-backend/internal/model"

	"gorm.io/gorm"
)

// CommandRepository 读写 commands 表。下发和回执尚未实现。
type CommandRepository interface {
	Find(ctx context.Context, commandID string) (*model.Command, error)
}

type commandRepo struct {
	db *gorm.DB
}

func NewCommandRepository(db *gorm.DB) CommandRepository {
	return &commandRepo{db: db}
}

func (r *commandRepo) Find(context.Context, string) (*model.Command, error) {
	_ = r.db
	return nil, apperr.ErrNotImplemented
}
