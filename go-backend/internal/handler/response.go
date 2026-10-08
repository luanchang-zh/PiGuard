// Package handler 是 HTTP 入口。
// 它负责状态码和响应信封，业务规则交给 service。成功时的字段映射等接口实现时再写。
package handler

import (
	"errors"
	"log/slog"
	"net/http"

	"piguard/go-backend/internal/apperr"

	"github.com/gin-gonic/gin"
)

// Response 是规范里的 HTTP 信封。失败时可以带 error 说明具体字段。
type Response struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
	Error   any    `json:"error,omitempty"`
}

func writeOK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, Response{Code: apperr.CodeOK, Message: "ok", Data: data})
}

// writeError 把服务层错误变成规范里的 code 和 message。
// 尚未实现使用 HTTP 501，避免和真正的 500 内部故障混在一起。
func writeError(c *gin.Context, err error) {
	for _, mapping := range []struct {
		err          error
		status, code int
		message      string
	}{
		{apperr.ErrCommandNotFound, http.StatusNotFound, apperr.CodeCommandNotFound, "command not found"},
		{apperr.ErrDeviceOffline, http.StatusServiceUnavailable, apperr.CodeDeviceOffline, "device offline"},
		{apperr.ErrMQTTUnavailable, http.StatusServiceUnavailable, apperr.CodeMQTTUnavailable, "MQTT unavailable"},
	} {
		if errors.Is(err, mapping.err) {
			c.JSON(mapping.status, Response{Code: mapping.code, Message: mapping.message})
			return
		}
	}
	if errors.Is(err, apperr.ErrDeviceNotFound) {
		c.JSON(http.StatusNotFound, Response{Code: apperr.CodeDeviceNotFound, Message: "device not found"})
		return
	}
	var invalid *apperr.InvalidParams
	if errors.As(err, &invalid) {
		c.JSON(http.StatusBadRequest, Response{Code: apperr.CodeInvalidParams, Message: "invalid parameter", Error: gin.H{"field": invalid.Field}})
		return
	}
	if errors.Is(err, apperr.ErrNotImplemented) {
		c.JSON(http.StatusNotImplemented, Response{
			Code:    apperr.CodeInternal,
			Message: "接口尚未实现",
		})
		return
	}
	slog.Error("请求处理失败", "err", err, "path", c.FullPath())
	c.JSON(http.StatusInternalServerError, Response{
		Code:    apperr.CodeInternal,
		Message: "内部错误",
	})
}
