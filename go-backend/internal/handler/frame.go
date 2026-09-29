package handler

import (
	"piguard/go-backend/internal/service"

	"github.com/gin-gonic/gin"
)

// FrameHandler 处理摄像头图片的上传和读取。
// 图片走 HTTP，不走 MQTT。文件落在截图目录，接口本身尚未实现。
type FrameHandler struct {
	svc service.FrameService
}

func NewFrameHandler(svc service.FrameService) *FrameHandler {
	return &FrameHandler{svc: svc}
}

// Upload 对应 POST /api/v1/devices/:device_id/frames，内容类型为 multipart/form-data。
func (h *FrameHandler) Upload(c *gin.Context) {
	if err := h.svc.Save(c.Request.Context(), c.Param("device_id")); err != nil {
		writeError(c, err)
		return
	}
	writeOK(c, gin.H{})
}

// Latest 对应 GET /api/v1/devices/:device_id/frame，返回最新预览的访问地址。
func (h *FrameHandler) Latest(c *gin.Context) {
	if err := h.svc.Latest(c.Request.Context(), c.Param("device_id")); err != nil {
		writeError(c, err)
		return
	}
	writeOK(c, gin.H{})
}

// Content 对应 GET /api/v1/snapshots/:snapshot_id/content，下一步会直接写出 JPEG。
func (h *FrameHandler) Content(c *gin.Context) {
	if err := h.svc.Open(c.Request.Context(), c.Param("snapshot_id")); err != nil {
		writeError(c, err)
		return
	}
	writeOK(c, gin.H{})
}
