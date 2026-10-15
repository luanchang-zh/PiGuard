package handler

import (
	"io"
	"net/http"
	"piguard/go-backend/internal/apperr"
	"piguard/go-backend/internal/protocol"
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
func (h *CommandHandler) Create(c *gin.Context) {
	raw, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10))
	if err != nil {
		writeError(c, &apperr.InvalidParams{Field: "body"})
		return
	}
	request, err := protocol.ParseCommandRequest(raw)
	if err != nil {
		writeError(c, &apperr.InvalidParams{Field: "body"})
		return
	}
	id, err := h.svc.Create(c.Request.Context(), c.Param("device_id"), *request)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, Response{Code: apperr.CodeOK, Message: "ok", Data: gin.H{"command_id": id, "status": "pending"}})
}

// Get 对应 GET /api/v1/commands/:command_id。
func (h *CommandHandler) Get(c *gin.Context) {
	row, err := h.svc.Get(c.Request.Context(), c.Param("command_id"))
	if err != nil {
		writeError(c, err)
		return
	}
	writeOK(c, row)
}
