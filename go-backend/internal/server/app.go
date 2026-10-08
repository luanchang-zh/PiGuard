// Package server 把配置、数据库、MQTT 和 HTTP 组装成一个进程。
// 依赖在这里创建并注入，业务包不自己寻找全局变量。
package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"piguard/go-backend/internal/config"
	"piguard/go-backend/internal/db"
	"piguard/go-backend/internal/handler"
	"piguard/go-backend/internal/mqtt"
	"piguard/go-backend/internal/realtime"
	"piguard/go-backend/internal/repo"
	"piguard/go-backend/internal/service"
)

// Run 启动平台，并在收到中断信号后关闭 HTTP 和 MQTT。
//
// 顺序是：准备截图目录、打开数据库并建表、写入演示设备、连接 Mosquitto、监听 HTTP。
// 连接 Broker 失败会返回错误，进程随之退出，避免没有消息通道时看起来仍在正常服务。
func Run(cfg *config.Config) error {
	if err := os.MkdirAll(cfg.Storage.SnapshotDir, 0o755); err != nil {
		return fmt.Errorf("创建截图目录: %w", err)
	}
	slog.Info("截图目录已就绪", "dir", cfg.Storage.SnapshotDir)

	gormDB, err := db.Open(cfg.Database.SQLitePath)
	if err != nil {
		return err
	}
	sqlDB, err := gormDB.DB()
	if err != nil {
		return fmt.Errorf("取得数据库连接: %w", err)
	}
	defer sqlDB.Close()
	slog.Info("数据库已打开", "path", cfg.Database.SQLitePath)

	deviceRepo := repo.NewDeviceRepository(gormDB)
	configRepo := repo.NewConfigRepository(gormDB)
	historyRepo := repo.NewTelemetryRepository(gormDB)
	hub := realtime.NewHub(32)
	defer hub.Close()
	monitor := service.NewMonitor(deviceRepo, historyRepo, hub)
	deviceSvc := service.NewDeviceService(deviceRepo, configRepo, monitor)
	seedResult, err := deviceSvc.Seed(context.Background(), service.SeedInput{
		DeviceID:        cfg.Seed.DeviceID,
		Name:            cfg.Seed.Name,
		Mode:            cfg.Seed.Mode,
		SoftwareVersion: cfg.Seed.SoftwareVersion,
	})
	if err != nil {
		return fmt.Errorf("写入演示设备: %w", err)
	}
	slog.Info("演示设备已就绪",
		"device_id", cfg.Seed.DeviceID,
		"device_created", seedResult.DeviceCreated,
		"config_created", seedResult.ConfigCreated,
	)
	if err := monitor.Disconnected(context.Background()); err != nil {
		return fmt.Errorf("初始化在线状态: %w", err)
	}
	monitorCtx, stopMonitor := context.WithCancel(context.Background())
	monitorDone := make(chan struct{})
	go func() { defer close(monitorDone); monitor.Run(monitorCtx) }()
	defer func() { stopMonitor(); <-monitorDone }()

	mqttClient := mqtt.NewClient(mqtt.Options{
		Broker:         cfg.MQTT.Broker,
		ClientID:       cfg.MQTT.ClientID,
		Username:       cfg.MQTT.Username,
		Password:       cfg.MQTT.Password,
		ConnectTimeout: cfg.MQTT.ConnectTimeout,
	})
	subscriber := mqtt.NewSubscriber(mqttClient, monitor)
	defer subscriber.Close()
	if err := mqttClient.Connect(); err != nil {
		return err
	}
	defer mqttClient.Disconnect()

	publisher := mqtt.NewPublisher(mqttClient)
	if err := subscriber.Start(); err != nil {
		return err
	}

	engine := handler.NewEngine(handler.Dependencies{
		Ping:      sqlDB.PingContext,
		Devices:   deviceSvc,
		Telemetry: service.NewTelemetryService(historyRepo, deviceRepo),
		Events:    service.NewEventService(repo.NewEventRepository(gormDB)),
		Commands:  service.NewCommandService(repo.NewCommandRepository(gormDB), publisher),
		Configs:   service.NewConfigService(configRepo, publisher),
		Frames:    service.NewFrameService(repo.NewSnapshotRepository(gormDB), cfg.Storage.SnapshotDir),
		Hub:       hub,
	})

	httpServer := &http.Server{
		Addr:              cfg.HTTP.Addr,
		Handler:           engine,
		ReadHeaderTimeout: 5 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() {
		slog.Info("HTTP 服务开始监听", "addr", cfg.HTTP.Addr)
		errCh <- httpServer.ListenAndServe()
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("HTTP 服务退出: %w", err)
		}
		return nil
	case sig := <-sigCh:
		slog.Info("收到退出信号，开始关闭", "signal", sig.String())
	}

	// 先停接收新请求，再由 defer 断开 MQTT、关闭数据库。
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	hub.Close()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("关闭 HTTP 服务: %w", err)
	}
	return nil
}
