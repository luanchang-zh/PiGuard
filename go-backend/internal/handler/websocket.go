package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"piguard/go-backend/internal/realtime"
)

type WebsocketHandler struct {
	hub   *realtime.Hub
	slots chan struct{}
}

func NewWebsocketHandler(hub *realtime.Hub) *WebsocketHandler {
	return &WebsocketHandler{hub: hub, slots: make(chan struct{}, 128)}
}

// Serve 对应 GET /api/v1/ws。
func (h *WebsocketHandler) Serve(c *gin.Context) {
	if h.hub == nil {
		c.Status(http.StatusServiceUnavailable)
		return
	}
	select {
	case h.slots <- struct{}{}:
		defer func() { <-h.slots }()
	default:
		c.JSON(http.StatusServiceUnavailable, Response{Code: 50001, Message: "websocket capacity reached"})
		return
	}
	// Origin and platform authorization have already passed the route guards.
	upgrader := websocket.Upgrader{ReadBufferSize: 1024, WriteBufferSize: 4096, CheckOrigin: func(*http.Request) bool { return true }}
	conn, err := upgrader.Upgrade(c.Writer, c.Request, c.Writer.Header())
	if err != nil {
		return
	}
	defer conn.Close()
	sub := h.hub.Subscribe()
	defer h.hub.Unsubscribe(sub)
	var sessionDone <-chan struct{}
	if s := sessionIdentity(c).Session; s != nil {
		sessionDone = s.Done
	}
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
			case <-sessionDone:
				return
			default:
			}
			select {
			case <-sessionDone:
				return
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
