package handler

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
)

// Health 检查进程和 SQLite。
// 启动时连不上 MQTT 会直接退出，所以这里不再把 Broker 当作健康条件。
func Health(ping func(context.Context) error) gin.HandlerFunc {
	return func(c *gin.Context) {
		if ping != nil {
			if err := ping(c.Request.Context()); err != nil {
				slog.Error("数据库健康检查失败", "err", err)
				c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unavailable"})
				return
			}
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	}
}
