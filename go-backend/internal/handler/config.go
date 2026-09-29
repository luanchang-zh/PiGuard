package handler

import (
	"piguard/go-backend/internal/service"

	"github.com/gin-gonic/gin"
)

// ConfigHandler 处理设备阈值的读取和修改。
type ConfigHandler struct {
	svc service.ConfigService
}

func NewConfigHandler(svc service.ConfigService) *ConfigHandler {
	return &ConfigHandler{svc: svc}
}

// Get 对应 GET /api/v1/devices/:device_id/config。
func (h *ConfigHandler) Get(c *gin.Context) {
	if _, err := h.svc.Get(c.Request.Context(), c.Param("device_id")); err != nil {
		writeError(c, err)
		return
	}
	writeOK(c, gin.H{})
}

// Update 对应 PUT /api/v1/devices/:device_id/config。
func (h *ConfigHandler) Update(c *gin.Context) {
	if err := h.svc.Update(c.Request.Context(), c.Param("device_id")); err != nil {
		writeError(c, err)
		return
	}
	writeOK(c, gin.H{})
}
