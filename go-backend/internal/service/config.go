package service

import (
	"context"
	"fmt"

	"piguard/go-backend/internal/apperr"
	"piguard/go-backend/internal/model"
	"piguard/go-backend/internal/repo"
)

// ConfigService 负责读取和更新某台设备的期望配置。
// 更新成功后应把配置发到 car/{device_id}/config，并等待设备回执。这一步还没做。
type ConfigService interface {
	Get(ctx context.Context, deviceID string) (*model.DeviceConfig, error)
	Update(ctx context.Context, deviceID string) error
}

type configService struct {
	configs repo.ConfigRepository
	pub     Publisher
}

func NewConfigService(configs repo.ConfigRepository, pub Publisher) ConfigService {
	return &configService{configs: configs, pub: pub}
}

// Get 暂不读取已经种下的默认阈值，配置查询接口留到下一步。
func (s *configService) Get(context.Context, string) (*model.DeviceConfig, error) {
	if s.configs == nil {
		return nil, fmt.Errorf("配置仓储未注入")
	}
	return nil, apperr.ErrNotImplemented
}

func (s *configService) Update(context.Context, string) error {
	if s.configs == nil || s.pub == nil {
		return fmt.Errorf("配置依赖未注入")
	}
	return apperr.ErrNotImplemented
}
