package model

import "time"

// Snapshot 记录一张已保存的摄像头图片，对应表 snapshots。
// JPEG 本体放在磁盘目录里，本表只保存路径和元数据。
type Snapshot struct {
	ID         uint   `gorm:"primaryKey"`
	SnapshotID string `gorm:"size:64;uniqueIndex;not null"`
	DeviceID   string `gorm:"size:64;index;uniqueIndex:idx_device_frame;index:idx_preview_order,priority:1;not null"`
	// Legacy rows may have NULL frame_id; new uploads always have an identity.
	FrameID *string `gorm:"size:64;uniqueIndex:idx_device_frame"`
	SHA256  string  `gorm:"size:64"`
	Size    int64
	// Separate seconds/nanoseconds preserve ordering across all RFC3339 years.
	CapturedSeconds    int64 `gorm:"index:idx_preview_order,priority:3,sort:desc"`
	CapturedNanosecond int   `gorm:"index:idx_preview_order,priority:4,sort:desc"`
	// Type 取值：preview（实时预览）、snapshot（命令拍照）、event（告警抓拍）。
	Type string `gorm:"size:32;index:idx_preview_order,priority:2;not null"`
	// Path 是 JPEG 在服务器本地的路径。
	Path       string    `gorm:"type:text;not null"`
	CapturedAt time.Time `gorm:"not null"`
	CreatedAt  time.Time
}

func (Snapshot) TableName() string { return "snapshots" }
