package repo

import (
	"context"
	"errors"

	"piguard/go-backend/internal/apperr"
	"piguard/go-backend/internal/model"

	"gorm.io/gorm"
)

// DeviceRepository 读写 devices 表。
// Find 和 Insert 给启动种子用。List 留给设备列表接口，当前固定返回尚未实现。
type DeviceRepository interface {
	Find(ctx context.Context, deviceID string) (*model.Device, error)
	Insert(ctx context.Context, device *model.Device) error
	List(ctx context.Context) ([]model.Device, error)
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

// List 先不查询全表。设备列表接口接通后再从这里返回数据。
func (r *deviceRepo) List(context.Context) ([]model.Device, error) {
	return nil, apperr.ErrNotImplemented
}
