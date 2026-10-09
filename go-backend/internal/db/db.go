// Package db 打开 SQLite，并按 model 自动建表。
// 表结构以 GORM AutoMigrate 为准，不另维护 SQL 迁移脚本。
package db

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"piguard/go-backend/internal/model"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Open 创建数据库文件（含父目录），迁移业务表，并限制为单连接。
// SQLite 同时只适合一个写连接，多连接并发写容易出现 database is locked。
func Open(path string) (*gorm.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("创建数据库目录: %w", err)
	}

	// 种子逻辑会先查询再插入，查不到是正常情况。
	// 忽略 record not found，避免第一次启动时把预期内的查询打成红色错误。
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{
		Logger: logger.New(log.New(os.Stdout, "", log.LstdFlags), logger.Config{
			SlowThreshold:             time.Second,
			LogLevel:                  logger.Warn,
			IgnoreRecordNotFoundError: true,
			Colorful:                  false,
		}),
	})
	if err != nil {
		return nil, fmt.Errorf("打开 SQLite %s: %w", path, err)
	}

	// 写锁等待 5 秒，避免启动种子和后续请求刚好叠在一起时立刻失败。
	if err := db.Exec("PRAGMA busy_timeout = 5000").Error; err != nil {
		return nil, fmt.Errorf("设置 SQLite busy_timeout: %w", err)
	}

	if err := db.AutoMigrate(
		&model.Device{},
		&model.DeviceConfig{},
		&model.ConfigRevision{},
		&model.Telemetry{},
		&model.Event{},
		&model.Command{},
		&model.Snapshot{},
	); err != nil {
		return nil, fmt.Errorf("自动建表: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("取得数据库连接: %w", err)
	}
	sqlDB.SetMaxOpenConns(1)
	return db, nil
}
