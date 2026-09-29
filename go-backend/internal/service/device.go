package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"piguard/go-backend/internal/apperr"
	"piguard/go-backend/internal/model"
	"piguard/go-backend/internal/repo"
)

// SeedInput 是启动时要保证存在的演示设备。
type SeedInput struct {
	DeviceID        string
	Name            string
	Mode            string
	SoftwareVersion string
}

// SeedResult 说明这次启动新建了哪些行。两边都为 false 表示设备已经登记过。
type SeedResult struct {
	DeviceCreated bool
	ConfigCreated bool
}

// DeviceService 负责设备登记，以及网页上的设备查询。
// List、Get、State 目前返回尚未实现，种子数据不会从这些接口漏出去。
type DeviceService interface {
	Seed(ctx context.Context, in SeedInput) (SeedResult, error)
	List(ctx context.Context) ([]model.Device, error)
	Get(ctx context.Context, deviceID string) (*model.Device, error)
	State(ctx context.Context, deviceID string) error
}

type deviceService struct {
	devices repo.DeviceRepository
	configs repo.ConfigRepository
}

func NewDeviceService(devices repo.DeviceRepository, configs repo.ConfigRepository) DeviceService {
	return &deviceService{devices: devices, configs: configs}
}

// Seed 保证演示设备和默认阈值各有一行。
// 期望配置版本从 1 开始，已确认版本保持 0：设备还没连上，不能把默认值写成已经生效。
// 再次启动时如果行已经在，就保持原样，不重置在线状态和后来改过的阈值。
func (s *deviceService) Seed(ctx context.Context, in SeedInput) (SeedResult, error) {
	var result SeedResult

	_, err := s.devices.Find(ctx, in.DeviceID)
	switch {
	case errors.Is(err, repo.ErrNotFound):
		device := &model.Device{
			DeviceID:              in.DeviceID,
			Name:                  in.Name,
			Online:                false,
			Mode:                  in.Mode,
			SoftwareVersion:       in.SoftwareVersion,
			ReportedConfigVersion: 0,
		}
		if err := s.devices.Insert(ctx, device); err != nil {
			return SeedResult{}, fmt.Errorf("登记设备 %s: %w", in.DeviceID, err)
		}
		result.DeviceCreated = true
	case err != nil:
		return SeedResult{}, err
	}

	_, err = s.configs.Find(ctx, in.DeviceID)
	switch {
	case errors.Is(err, repo.ErrNotFound):
		raw, err := json.Marshal(model.DefaultRules())
		if err != nil {
			return SeedResult{}, fmt.Errorf("序列化默认阈值: %w", err)
		}
		cfg := &model.DeviceConfig{
			DeviceID:        in.DeviceID,
			DesiredVersion:  1,
			ReportedVersion: 0,
			ConfigJSON:      string(raw),
		}
		if err := s.configs.Insert(ctx, cfg); err != nil {
			return SeedResult{}, fmt.Errorf("写入默认阈值: %w", err)
		}
		result.ConfigCreated = true
	case err != nil:
		return SeedResult{}, err
	}

	return result, nil
}

func (s *deviceService) List(ctx context.Context) ([]model.Device, error) {
	return s.devices.List(ctx)
}

// Get 暂不调用仓储的 Find。Find 目前只服务于启动种子。
func (s *deviceService) Get(context.Context, string) (*model.Device, error) {
	if s.devices == nil {
		return nil, fmt.Errorf("设备仓储未注入")
	}
	return nil, apperr.ErrNotImplemented
}

// State 应返回车速、距离、温度、角速度、车道和风险等级。数据来自最新遥测，尚未接入。
func (s *deviceService) State(context.Context, string) error {
	if s.devices == nil {
		return fmt.Errorf("设备仓储未注入")
	}
	return apperr.ErrNotImplemented
}
