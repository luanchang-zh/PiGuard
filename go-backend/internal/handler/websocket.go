package handler

import (
	"net/http"
	"piguard/go-backend/internal/realtime"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

type WebsocketHandler struct{ hub *realtime.Hub }

func NewWebsocketHandler(hub *realtime.Hub) *WebsocketHandler {
	return &WebsocketHandler{hub: hub}
}

// Serve 对应 GET /api/v1/ws。
func (h *WebsocketHandler) Serve(c *gin.Context) {
	if h.hub == nil {
		c.Status(http.StatusServiceUnavailable)
		return
	}
	upgrader := websocket.Upgrader{ReadBufferSize: 1024, WriteBufferSize: 4096}
	sub := h.hub.Subscribe()
	defer h.hub.Unsubscribe(sub)
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	conn.SetReadLimit(4096)
	_ = conn.SetReadDeadline(time.Now().Add(45 * time.Second))
	conn.SetPongHandler(func(string) error { return conn.SetReadDeadline(time.Now().Add(45 * time.Second)) })
	written := make(chan struct{})
	go func() {
		defer close(written)
		defer conn.Close()
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-sub.Done:
				return
			case data := <-sub.Messages:
				_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
				if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
					return
				}
			case <-ticker.C:
				_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
				if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					return
				}
			}
		}
	}()
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			break
		}
	}
	h.hub.Unsubscribe(sub)
	<-written
}
