package handler

import (
	"piguard/go-backend/internal/service"

	"github.com/gin-gonic/gin"
)

// CommandHandler 处理下行命令的创建和状态查询。
type CommandHandler struct {
	svc service.CommandService
}

func NewCommandHandler(svc service.CommandService) *CommandHandler {
	return &CommandHandler{svc: svc}
}

// Create 对应 POST /api/v1/devices/:device_id/commands。
// 请求体里的 type 和 params 在实现时再读取。
func (h *CommandHandler) Create(c *gin.Context) {
	if err := h.svc.Create(c.Request.Context(), c.Param("device_id")); err != nil {
		writeError(c, err)
		return
	}
	writeOK(c, gin.H{})
}

// Get 对应 GET /api/v1/commands/:command_id。
func (h *CommandHandler) Get(c *gin.Context) {
	if _, err := h.svc.Get(c.Request.Context(), c.Param("command_id")); err != nil {
		writeError(c, err)
		return
	}
	writeOK(c, gin.H{})
}
