package repo

import (
	"context"
	"errors"

	"piguard/go-backend/internal/model"

	"gorm.io/gorm"
)

// ConfigRepository 读写 device_configs 表。
// 启动时用 Find 判断演示配置是否已经存在，避免重复插入。
type ConfigRepository interface {
	Find(ctx context.Context, deviceID string) (*model.DeviceConfig, error)
	Insert(ctx context.Context, cfg *model.DeviceConfig) error
}

type configRepo struct {
	db *gorm.DB
}

func NewConfigRepository(db *gorm.DB) ConfigRepository {
	return &configRepo{db: db}
}

func (r *configRepo) Find(ctx context.Context, deviceID string) (*model.DeviceConfig, error) {
	var cfg model.DeviceConfig
	err := r.db.WithContext(ctx).Where("device_id = ?", deviceID).First(&cfg).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (r *configRepo) Insert(ctx context.Context, cfg *model.DeviceConfig) error {
	return r.db.WithContext(ctx).Create(cfg).Error
}
