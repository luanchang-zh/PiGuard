package repo

import (
	"context"
	"errors"

	"piguard/go-backend/internal/model"

	"gorm.io/gorm"
)

// DeviceRepository 读写 devices 表。
type DeviceRepository interface {
	Find(ctx context.Context, deviceID string) (*model.Device, error)
	Insert(ctx context.Context, device *model.Device) error
	List(ctx context.Context) ([]model.Device, error)
	Update(ctx context.Context, deviceID string, fields map[string]any) error
	MarkAllOffline(ctx context.Context) error
}

type deviceRepo struct {
	db *gorm.DB
}

func NewDeviceRepository(db *gorm.DB) DeviceRepository {
	return &deviceRepo{db: db}
}

func (r *deviceRepo) Find(ctx context.Context, deviceID string) (*model.Device, error) {
	var device model.Device
	err := r.db.WithContext(ctx).Where("device_id = ?", deviceID).First(&device).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &device, nil
}

func (r *deviceRepo) Insert(ctx context.Context, device *model.Device) error {
	return r.db.WithContext(ctx).Create(device).Error
}

func (r *deviceRepo) List(ctx context.Context) ([]model.Device, error) {
	devices := []model.Device{}
	err := r.db.WithContext(ctx).Order("device_id ASC").Find(&devices).Error
	return devices, err
}

func (r *deviceRepo) Update(ctx context.Context, deviceID string, fields map[string]any) error {
	return r.db.WithContext(ctx).Model(&model.Device{}).Where("device_id = ?", deviceID).Updates(fields).Error
}

func (r *deviceRepo) MarkAllOffline(ctx context.Context) error {
	return r.db.WithContext(ctx).Model(&model.Device{}).Where("online = ?", true).Update("online", false).Error
}
