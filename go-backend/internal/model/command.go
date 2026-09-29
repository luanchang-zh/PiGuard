package model

import "time"

// Command 是平台下发的一条命令，对应表 commands。
// MQTT 发布成功只表示消息到了 Broker，Status 要等设备回执后才变成 success 或 failed。
type Command struct {
	ID        uint   `gorm:"primaryKey"`
	CommandID string `gorm:"size:64;uniqueIndex;not null"`
	DeviceID  string `gorm:"size:64;index;not null"`
	// Type 例如 buzzer.test、indicator.test、camera.snapshot、scenario.start、scenario.stop、config.update。
	Type string `gorm:"size:64;not null"`
	// Params 是命令参数的 JSON 原文。
	Params string `gorm:"type:text"`
	// Status 取值：pending、sent、success、failed、timeout。
	Status    string    `gorm:"size:16;index;not null"`
	IssuedAt  time.Time `gorm:"not null"`
	ExpiresAt time.Time `gorm:"not null"`
	AckAt     *time.Time
	// Result 保存设备回执里的结果 JSON，例如拍照返回的 snapshot_id。
	Result string `gorm:"type:text"`
	Error  string `gorm:"type:text"`
}

func (Command) TableName() string { return "commands" }
