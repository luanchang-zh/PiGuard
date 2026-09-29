package handler

import (
	"piguard/go-backend/internal/service"

	"github.com/gin-gonic/gin"
)

// TelemetryHandler 处理历史遥测查询。
type TelemetryHandler struct {
	svc service.TelemetryService
}

func NewTelemetryHandler(svc service.TelemetryService) *TelemetryHandler {
	return &TelemetryHandler{svc: svc}
}

// List 对应 GET /api/v1/devices/:device_id/telemetry。
// 时间范围和 limit 等查询参数在实现时再解析。
func (h *TelemetryHandler) List(c *gin.Context) {
	if _, err := h.svc.List(c.Request.Context(), c.Param("device_id")); err != nil {
		writeError(c, err)
		return
	}
	writeOK(c, gin.H{"items": []any{}})
}
