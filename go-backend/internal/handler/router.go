package handler

import (
	"context"

	"piguard/go-backend/internal/realtime"
	"piguard/go-backend/internal/service"

	"github.com/gin-gonic/gin"
)

// Dependencies 是 HTTP 层需要的依赖。由启动流程创建具体实现后注入。
// Ping 用于健康检查；监控业务共用一个实时 Hub。
type Dependencies struct {
	Ping      func(context.Context) error
	Devices   service.DeviceService
	Telemetry service.TelemetryService
	Events    service.EventService
	Commands  service.CommandService
	Configs   service.ConfigService
	Frames    service.FrameService
	Hub       *realtime.Hub
}

// NewEngine 注册健康检查和规范中的 /api/v1 路由。
// 图片上传与内容读取和监控共用同一个 Hub。
func NewEngine(dep Dependencies) *gin.Engine {
	engine := gin.New()
	engine.Use(gin.Logger(), gin.Recovery())
	engine.GET("/health", Health(dep.Ping))

	devices := NewDeviceHandler(dep.Devices)
	telemetry := NewTelemetryHandler(dep.Telemetry)
	events := NewEventHandler(dep.Events)
	commands := NewCommandHandler(dep.Commands)
	configs := NewConfigHandler(dep.Configs)
	frames := NewFrameHandler(dep.Frames)
	ws := NewWebsocketHandler(dep.Hub)

	v1 := engine.Group("/api/v1")
	v1.GET("/ws", ws.Serve)
	v1.GET("/commands/:command_id", commands.Get)
	v1.GET("/snapshots/:snapshot_id/content", frames.Content)

	deviceRoutes := v1.Group("/devices")
	deviceRoutes.GET("", devices.List)
	deviceRoutes.GET("/:device_id", devices.Get)
	deviceRoutes.GET("/:device_id/state", devices.State)
	deviceRoutes.GET("/:device_id/telemetry", telemetry.List)
	deviceRoutes.GET("/:device_id/events", events.List)
	deviceRoutes.GET("/:device_id/config", configs.Get)
	deviceRoutes.PUT("/:device_id/config", configs.Update)
	deviceRoutes.POST("/:device_id/commands", commands.Create)
	deviceRoutes.POST("/:device_id/frames", frames.Upload)
	deviceRoutes.GET("/:device_id/frame", frames.Latest)

	return engine
}
