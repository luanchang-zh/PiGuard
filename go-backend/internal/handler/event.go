package handler

import (
	"piguard/go-backend/internal/service"

	"github.com/gin-gonic/gin"
)

// EventHandler 处理告警记录查询。
type EventHandler struct {
	svc service.EventService
}

func NewEventHandler(svc service.EventService) *EventHandler {
	return &EventHandler{svc: svc}
}

// List 对应 GET /api/v1/devices/:device_id/events。
func (h *EventHandler) List(c *gin.Context) {
	if _, err := h.svc.List(c.Request.Context(), c.Param("device_id")); err != nil {
		writeError(c, err)
		return
	}
	writeOK(c, gin.H{"items": []any{}})
}
