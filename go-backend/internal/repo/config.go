package repo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"piguard/go-backend/internal/apperr"
	"piguard/go-backend/internal/model"
	"piguard/go-backend/internal/protocol"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ConfigRepository 读写 device_configs 表。
// 启动时用 Find 判断演示配置是否已经存在，避免重复插入。
type ConfigRepository interface {
	Find(ctx context.Context, deviceID string) (*model.DeviceConfig, error)
	Insert(ctx context.Context, cfg *model.DeviceConfig) error
	Snapshot(context.Context, string) (*ConfigSnapshot, error)
	UpdateDesired(context.Context, string, protocol.ConfigRequest, time.Time) (int, error)
	EnsureRevisions(context.Context) error
	ListDeviceIDs(context.Context) ([]string, error)
	ApplyAck(context.Context, protocol.ConfigAck, time.Time) (bool, error)
}

type ConfigSnapshot struct {
	Config   model.DeviceConfig
	Revision model.ConfigRevision
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

func snapshot(tx *gorm.DB, id string) (*ConfigSnapshot, error) {
	var result ConfigSnapshot
	if err := tx.Where("device_id = ?", id).First(&result.Config).Error; err != nil {
		return nil, err
	}
	err := tx.Where("device_id = ? AND config_version = ?", id, result.Config.DesiredVersion).First(&result.Revision).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		// GET can precede Start; an unissued seed is pending, never confirmed.
		result.Revision = model.ConfigRevision{DeviceID: id, ConfigVersion: result.Config.DesiredVersion, ConfigJSON: result.Config.ConfigJSON, IssuedAt: result.Config.UpdatedAt, Status: "pending"}
	} else if err != nil {
		return nil, err
	}
	return &result, nil
}
func (r *configRepo) Snapshot(ctx context.Context, id string) (*ConfigSnapshot, error) {
	var result *ConfigSnapshot
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error { var err error; result, err = snapshot(tx, id); return err })
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return result, err
}
func ensureRevision(tx *gorm.DB, c model.DeviceConfig) error {
	row := model.ConfigRevision{DeviceID: c.DeviceID, ConfigVersion: c.DesiredVersion, ConfigJSON: c.ConfigJSON, IssuedAt: c.UpdatedAt.UTC(), Status: "pending"}
	return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "device_id"}, {Name: "config_version"}}, DoNothing: true}).Create(&row).Error
}
func (r *configRepo) EnsureRevisions(ctx context.Context) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var configs []model.DeviceConfig
		if err := tx.Joins("JOIN devices ON devices.device_id = device_configs.device_id").Find(&configs).Error; err != nil {
			return err
		}
		for _, c := range configs {
			if err := ensureRevision(tx, c); err != nil {
				return err
			}
		}
		return nil
	})
}
func (r *configRepo) ListDeviceIDs(ctx context.Context) ([]string, error) {
	ids := []string{}
	err := r.db.WithContext(ctx).Model(&model.DeviceConfig{}).Joins("JOIN devices ON devices.device_id = device_configs.device_id").Order("device_configs.device_id ASC").Pluck("device_configs.device_id", &ids).Error
	return ids, err
}
func (r *configRepo) UpdateDesired(ctx context.Context, id string, request protocol.ConfigRequest, at time.Time) (int, error) {
	var version int
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current model.DeviceConfig
		if err := tx.Where("device_id = ?", id).First(&current).Error; err != nil {
			return err
		}
		if current.DesiredVersion != request.ExpectedVersion {
			return apperr.ErrConfigVersionConflict
		}
		var document model.RulesConfig
		if err := json.Unmarshal([]byte(current.ConfigJSON), &document); err != nil {
			return err
		}
		rules, err := request.Merge(document.Rules)
		if err != nil {
			return err
		}
		if err = ensureRevision(tx, current); err != nil {
			return err
		}
		encoded, err := json.Marshal(model.RulesConfig{Rules: rules})
		if err != nil {
			return err
		}
		version = current.DesiredVersion + 1
		result := tx.Model(&model.DeviceConfig{}).Where("device_id = ? AND desired_version = ?", id, request.ExpectedVersion).Updates(map[string]any{"desired_version": version, "config_json": string(encoded), "updated_at": at})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return apperr.ErrConfigVersionConflict
		}
		return tx.Create(&model.ConfigRevision{DeviceID: id, ConfigVersion: version, ConfigJSON: string(encoded), IssuedAt: at.UTC(), Status: "pending"}).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, ErrNotFound
	}
	return version, err
}
func (r *configRepo) ApplyAck(ctx context.Context, ack protocol.ConfigAck, at time.Time) (bool, error) {
	changed := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row model.ConfigRevision
		if err := tx.Where("device_id = ? AND config_version = ?", ack.DeviceID, ack.ConfigVersion).First(&row).Error; err != nil {
			return err
		}
		errJSON := ""
		if ack.Error != nil {
			raw, err := json.Marshal(ack.Error)
			if err != nil {
				return err
			}
			errJSON = string(raw)
		}
		if row.Status != "pending" {
			matchingTime := (row.AppliedAt == nil && ack.AppliedAt == nil) || (row.AppliedAt != nil && ack.AppliedAt != nil && row.AppliedAt.Equal(*ack.AppliedAt))
			if row.Status != ack.Status || row.ErrorJSON != errJSON || !matchingTime {
				return fmt.Errorf("conflicting config Ack for %s/%d", ack.DeviceID, ack.ConfigVersion)
			}
			return nil
		}
		result := tx.Model(&model.ConfigRevision{}).Where("id = ? AND status = ?", row.ID, "pending").Updates(map[string]any{"status": ack.Status, "ack_at": at.UTC(), "applied_at": ack.AppliedAt, "error_json": errJSON})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return nil
		}
		if ack.Status == "success" {
			result = tx.Model(&model.DeviceConfig{}).Where("device_id = ?", ack.DeviceID).Update("reported_version", gorm.Expr("CASE WHEN reported_version < ? THEN ? ELSE reported_version END", ack.ConfigVersion, ack.ConfigVersion))
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return ErrNotFound
			}
		}
		changed = true
		return nil
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, ErrNotFound
	}
	return changed && err == nil, err
}
