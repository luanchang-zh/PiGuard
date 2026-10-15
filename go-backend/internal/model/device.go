package model

import "time"

// Device 是平台登记的一台边缘设备，对应表 devices。
// 对外标识用 DeviceID（例如 car-001），自增 ID 只给数据库内部使用。
type Device struct {
	ID uint `gorm:"primaryKey"`
	// DeviceID 与 MQTT 主题、HTTP 路径中的设备标识相同。
	DeviceID string `gorm:"size:64;uniqueIndex;not null"`
	Name     string `gorm:"size:128;not null"`
	// Online 由设备 status 和遗嘱消息维护。种子数据写入时为离线。
	Online bool `gorm:"not null;default:false"`
	// Mode 来自设备上报，例如 mock 或 hardware。种子默认写成 mock。
	Mode            string `gorm:"size:32;not null"`
	SoftwareVersion string `gorm:"size:32;not null"`
	// ReportedConfigVersion 是设备已经确认生效的配置版本。
	// 平台希望设备使用的版本放在 device_configs.desired_version，两张表分开记。
	ReportedConfigVersion int `gorm:"not null;default:0"`
	LastSeenAt            *time.Time
	SensorsJSON           string `gorm:"type:text"`
	StatusJSON            string `gorm:"type:text"`
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

func (Device) TableName() string { return "devices" }
