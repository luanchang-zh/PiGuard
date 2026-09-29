package handler

import (
	"piguard/go-backend/internal/apperr"

	"github.com/gin-gonic/gin"
)

// WebsocketHandler 预留给浏览器的实时推送。
// 消息类型将是 telemetry、event、device_status、command_update、config_update、frame_update。
// 浏览器不直接连接 MQTT。连接升级和广播还没做，所以这里还没有单独的 service。
type WebsocketHandler struct{}

func NewWebsocketHandler() *WebsocketHandler {
	return &WebsocketHandler{}
}

// Serve 对应 GET /api/v1/ws。
func (h *WebsocketHandler) Serve(c *gin.Context) {
	writeError(c, apperr.ErrNotImplemented)
}
