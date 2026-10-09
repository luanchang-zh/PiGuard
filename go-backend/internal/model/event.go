package model

import "time"

// Event 是一条告警或设备异常，对应表 events。
// 同一 event_id 只应入库一次，幂等检查留到事件接入时再做。
type Event struct {
	ID       uint   `gorm:"primaryKey"`
	EventID  string `gorm:"size:64;uniqueIndex;not null"`
	DeviceID string `gorm:"size:64;index;not null"`
	// Type 取值见规范：obstacle_warning、lane_departure、sharp_turn、high_temperature、sensor_failure。
	Type string `gorm:"size:64;index;not null"`
	// Action 取 started、level_changed、recovered。每条 MQTT 消息单独成行，不回写旧记录。
	Action string `gorm:"size:32;not null"`
	Level  string `gorm:"size:16;not null"`
	// Payload 保存事件 data 对象的 JSON 原文。不同告警的字段不一样，不拆成固定列。
	Payload    string    `gorm:"type:text"`
	Timestamp  time.Time `gorm:"not null"`
	SnapshotID string    `gorm:"size:64"`
}

func (Event) TableName() string { return "events" }
