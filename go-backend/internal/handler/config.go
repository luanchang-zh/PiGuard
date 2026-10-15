package handler

import (
	"io"
	"net/http"
	"piguard/go-backend/internal/apperr"
	"piguard/go-backend/internal/protocol"
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
	view, err := h.svc.Get(c.Request.Context(), c.Param("device_id"))
	if err != nil {
		writeError(c, err)
		return
	}
	writeOK(c, view)
}

// Update 对应 PUT /api/v1/devices/:device_id/config。
func (h *ConfigHandler) Update(c *gin.Context) {
	raw, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10))
	if err != nil {
		writeError(c, &apperr.InvalidParams{Field: "body"})
		return
	}
	request, err := protocol.ParseConfigRequest(raw)
	if err != nil {
		writeError(c, err)
		return
	}
	version, err := h.svc.Update(c.Request.Context(), c.Param("device_id"), *request)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, Response{Code: apperr.CodeOK, Message: "ok", Data: gin.H{"config_version": version, "status": "pending"}})
}
