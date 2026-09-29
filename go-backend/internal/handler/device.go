package handler

import (
	"piguard/go-backend/internal/service"

	"github.com/gin-gonic/gin"
)

// DeviceHandler 处理设备列表、设备详情和最新状态。
type DeviceHandler struct {
	svc service.DeviceService
}

func NewDeviceHandler(svc service.DeviceService) *DeviceHandler {
	return &DeviceHandler{svc: svc}
}

// List 对应 GET /api/v1/devices。
func (h *DeviceHandler) List(c *gin.Context) {
	if _, err := h.svc.List(c.Request.Context()); err != nil {
		writeError(c, err)
		return
	}
	// 查询接通后，在这里把 model.Device 映射成规范里的字段，不直接返回表结构。
	writeOK(c, []any{})
}

// Get 对应 GET /api/v1/devices/:device_id。
func (h *DeviceHandler) Get(c *gin.Context) {
	if _, err := h.svc.Get(c.Request.Context(), c.Param("device_id")); err != nil {
		writeError(c, err)
		return
	}
	writeOK(c, gin.H{})
}

// State 对应 GET /api/v1/devices/:device_id/state。
func (h *DeviceHandler) State(c *gin.Context) {
	if err := h.svc.State(c.Request.Context(), c.Param("device_id")); err != nil {
		writeError(c, err)
		return
	}
	writeOK(c, gin.H{})
}
