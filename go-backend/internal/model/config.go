package model

import "time"

// DeviceConfig 保存某台设备的期望配置，对应表 device_configs。
// DesiredVersion 由平台递增；ReportedVersion 只在收到设备配置回执后前进。
type DeviceConfig struct {
	ID              uint   `gorm:"primaryKey"`
	DeviceID        string `gorm:"size:64;uniqueIndex;not null"`
	DesiredVersion  int    `gorm:"not null"`
	ReportedVersion int    `gorm:"not null;default:0"`
	// ConfigJSON 是完整规则文档，结构见 RulesConfig。
	ConfigJSON string `gorm:"type:text;not null"`
	UpdatedAt  time.Time
}

func (DeviceConfig) TableName() string { return "device_configs" }

// RulesConfig 是配置文档的根。网页修改阈值时提交其中的 rules 对象。
type RulesConfig struct {
	Rules RuleSet `json:"rules"`
}

// RuleSet 覆盖四类预警。字段名与边缘端 config.yaml 里的规则一致。
type RuleSet struct {
	Obstacle      ObstacleRule      `json:"obstacle"`
	LaneDeparture LaneDepartureRule `json:"lane_departure"`
	SharpTurn     SharpTurnRule     `json:"sharp_turn"`
	Temperature   TemperatureRule   `json:"temperature"`
}

// ObstacleRule 用前方距离划分警告和危险，单位是米。
type ObstacleRule struct {
	WarningDistanceM float64 `json:"warning_distance_m"`
	DangerDistanceM  float64 `json:"danger_distance_m"`
}

// LaneDepartureRule 用归一化车道偏移判断偏离。OffsetThreshold 无量纲，DurationMS 是持续时间。
type LaneDepartureRule struct {
	OffsetThreshold float64 `json:"offset_threshold"`
	DurationMS      int     `json:"duration_ms"`
}

// SharpTurnRule 同时看角速度和车速，避免低速掉头误报。
type SharpTurnRule struct {
	YawThresholdDegS float64 `json:"yaw_threshold_deg_s"`
	MinSpeedKmh      float64 `json:"min_speed_kmh"`
	DurationMS       int     `json:"duration_ms"`
}

// TemperatureRule 使用触发阈值和恢复阈值，中间区间保持原状态，减少阈值附近抖动。
type TemperatureRule struct {
	TriggerC   float64 `json:"trigger_c"`
	RecoverC   float64 `json:"recover_c"`
	DurationMS int     `json:"duration_ms"`
}

// DefaultRules 返回课程设计里的初始阈值，供首次启动写入演示设备。
func DefaultRules() RulesConfig {
	return RulesConfig{
		Rules: RuleSet{
			Obstacle: ObstacleRule{
				WarningDistanceM: 15,
				DangerDistanceM:  8,
			},
			LaneDeparture: LaneDepartureRule{
				OffsetThreshold: 0.35,
				DurationMS:      1000,
			},
			SharpTurn: SharpTurnRule{
				YawThresholdDegS: 30,
				MinSpeedKmh:      20,
				DurationMS:       300,
			},
			Temperature: TemperatureRule{
				TriggerC:   35,
				RecoverC:   33,
				DurationMS: 5000,
			},
		},
	}
}
