package model

import "time"

// Telemetry 是平台保存的一条历史快照，对应表 telemetry。
// 边缘端陀螺仪可以到 50Hz，这里只准备存放降频后的记录，降频逻辑尚未实现。
type Telemetry struct {
	ID       uint   `gorm:"primaryKey"`
	DeviceID string `gorm:"size:64;index:idx_telemetry_device_time,priority:1;not null"`
	// Timestamp 是这条快照在设备侧的采样时间，不是平台入库时间。
	Timestamp time.Time `gorm:"index:idx_telemetry_device_time,priority:2;not null"`
	// Seq 是设备侧递增序号，用来丢掉乱序或回退的旧消息。
	Seq         uint64 `gorm:"not null"`
	Speed       float64
	Distance    float64
	Temperature float64
	GyroX       float64
	GyroY       float64
	GyroZ       float64
	YawRate     float64
	LaneOffset  float64
	RiskLevel   string `gorm:"size:16"`
}

func (Telemetry) TableName() string { return "telemetry" }
