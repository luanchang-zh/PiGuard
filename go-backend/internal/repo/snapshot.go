package repo

import (
	"context"

	"piguard/go-backend/internal/apperr"
	"piguard/go-backend/internal/model"

	"gorm.io/gorm"
)

// SnapshotRepository 读写 snapshots 表。图片文件本身不进数据库。
type SnapshotRepository interface {
	Find(ctx context.Context, snapshotID string) (*model.Snapshot, error)
	FindLatest(ctx context.Context, deviceID string) (*model.Snapshot, error)
}

type snapshotRepo struct {
	db *gorm.DB
}

func NewSnapshotRepository(db *gorm.DB) SnapshotRepository {
	return &snapshotRepo{db: db}
}

func (r *snapshotRepo) Find(context.Context, string) (*model.Snapshot, error) {
	_ = r.db
	return nil, apperr.ErrNotImplemented
}

func (r *snapshotRepo) FindLatest(context.Context, string) (*model.Snapshot, error) {
	_ = r.db
	return nil, apperr.ErrNotImplemented
}
