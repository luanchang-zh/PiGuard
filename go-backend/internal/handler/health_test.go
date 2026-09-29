package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestHealth 覆盖进程健康检查：数据库可 ping 时返回 ok，ping 失败时返回 503。
func TestHealth(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("数据库可连接", func(t *testing.T) {
		engine := NewEngine(Dependencies{
			Ping: func(context.Context) error { return nil },
		})
		status, body := getJSON(engine, "/health")
		if status != http.StatusOK || body != `{"status":"ok"}` {
			t.Fatalf("健康检查应返回 200 和 ok，得到 %d %s", status, body)
		}
	})

	t.Run("数据库不可用", func(t *testing.T) {
		engine := NewEngine(Dependencies{
			Ping: func(context.Context) error { return errors.New("db down") },
		})
		status, body := getJSON(engine, "/health")
		if status != http.StatusServiceUnavailable || body != `{"status":"unavailable"}` {
			t.Fatalf("数据库失败时应返回 503，得到 %d %s", status, body)
		}
	})
}

func getJSON(engine http.Handler, path string) (int, string) {
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	engine.ServeHTTP(recorder, req)
	return recorder.Code, recorder.Body.String()
}
