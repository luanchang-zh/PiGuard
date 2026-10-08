package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"gorm.io/gorm"
	"piguard/go-backend/internal/db"
	"piguard/go-backend/internal/model"
	"piguard/go-backend/internal/protocol"
	"piguard/go-backend/internal/realtime"
	"piguard/go-backend/internal/repo"
	"piguard/go-backend/internal/service"
	"piguard/go-backend/internal/simulator"
	"piguard/go-backend/internal/testutil"
)

type simulatedCommandPublisher struct {
	available atomic.Bool
	executor  *simulator.CommandExecutor
	manager   *service.CommandManager
}

func (p *simulatedCommandPublisher) Available() bool { return p.available.Load() }
func (p *simulatedCommandPublisher) Publish(ctx context.Context, topic string, qos byte, retained bool, raw []byte) error {
	if qos != 1 || retained {
		return fmt.Errorf("incorrect command QoS/retain")
	}
	ack, _, err := p.executor.Execute(topic, raw, time.Now().UTC())
	if err != nil {
		return err
	}
	if ack == nil {
		return nil
	}
	payload, err := json.Marshal(ack)
	if err != nil {
		return err
	}
	return p.manager.IngestAck(ctx, "car/"+ack.DeviceID+"/command-acks", payload)
}

type commandHTTPFixture struct {
	server    *httptest.Server
	database  *gorm.DB
	monitor   *service.Monitor
	publisher *simulatedCommandPublisher
}

func commandHTTP(t *testing.T, mode string) *commandHTTPFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)
	g, err := db.Open(filepath.Join(t.TempDir(), "http-commands.db"))
	if err != nil {
		t.Fatal(err)
	}
	sql, _ := g.DB()
	t.Cleanup(func() { sql.Close() })
	d, h := repo.NewDeviceRepository(g), repo.NewTelemetryRepository(g)
	hub := realtime.NewHub(32)
	t.Cleanup(hub.Close)
	m := service.NewMonitor(d, h, hub)
	devices := service.NewDeviceService(d, repo.NewConfigRepository(g), m)
	if _, err := devices.Seed(context.Background(), service.SeedInput{DeviceID: "car-001", Name: "Demo", Mode: "mock", SoftwareVersion: "0.1.0"}); err != nil {
		t.Fatal(err)
	}
	if err := m.Ingest(context.Background(), "car/car-001/status", testutil.JSON(testutil.Status("car-001", time.Now().UTC(), true))); err != nil {
		t.Fatal(err)
	}
	executor, err := simulator.NewCommandExecutor(mode)
	if err != nil {
		t.Fatal(err)
	}
	p := &simulatedCommandPublisher{executor: executor}
	p.available.Store(true)
	s := service.NewCommandService(repo.NewCommandRepository(g), d, p, m.Online, hub)
	p.manager = s
	if err := s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	srv := httptest.NewServer(NewEngine(Dependencies{Ping: sql.PingContext, Devices: devices, Telemetry: service.NewTelemetryService(h, d), Commands: s, Hub: hub}))
	t.Cleanup(srv.Close)
	return &commandHTTPFixture{server: srv, database: g, monitor: m, publisher: p}
}
func commandHTTPCall(t *testing.T, f *commandHTTPFixture, method, path, body string) (int, Response) {
	t.Helper()
	request, err := http.NewRequest(method, f.server.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	client := http.Client{Timeout: 3 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	var envelope Response
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatal(string(raw), err)
	}
	return response.StatusCode, envelope
}
func viewHTTP(t *testing.T, f *commandHTTPFixture, id, status string) protocol.CommandView {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		code, r := commandHTTPCall(t, f, "GET", "/api/v1/commands/"+id, "")
		if code != 200 || r.Code != 0 {
			t.Fatal(code, r)
		}
		raw, _ := json.Marshal(r.Data)
		var view protocol.CommandView
		if err := json.Unmarshal(raw, &view); err != nil {
			t.Fatal(err)
		}
		if view.Status == status {
			return view
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("missing status", status)
	return protocol.CommandView{}
}
func commandWS(t *testing.T, f *commandHTTPFixture) *websocket.Conn {
	t.Helper()
	c, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(f.server.URL, "http")+"/api/v1/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}
func commandWSEvent(t *testing.T, c *websocket.Conn, id, status string) protocol.CommandView {
	t.Helper()
	c.SetReadDeadline(time.Now().Add(3 * time.Second))
	for {
		var event struct {
			Type      string               `json:"type"`
			Timestamp time.Time            `json:"timestamp"`
			Data      protocol.CommandView `json:"data"`
		}
		if err := c.ReadJSON(&event); err != nil {
			t.Fatal(err)
		}
		if event.Type == "command_update" && event.Data.CommandID == id && event.Data.Status == status {
			if event.Timestamp.IsZero() {
				t.Fatal("missing envelope time")
			}
			return event.Data
		}
	}
}

func equalJSON(t *testing.T, left, right json.RawMessage) bool {
	t.Helper()
	var a, b any
	if err := json.Unmarshal(left, &a); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(right, &b); err != nil {
		t.Fatal(err)
	}
	return reflect.DeepEqual(a, b)
}

func TestCommandsHTTPAndTwoWebsockets(t *testing.T) {
	f := commandHTTP(t, "success")
	clients := []*websocket.Conn{commandWS(t, f), commandWS(t, f)}
	ids := map[string]bool{}
	for _, body := range []string{
		`{"type":"buzzer.test","params":{"duration_ms":1000}}`,
		`{"type":"indicator.test","params":{"color":"yellow","duration_ms":2000}}`,
		`{"type":"scenario.start","params":{"scenario":"obstacle_approach","speed":1.0}}`,
		`{"type":"scenario.stop","params":{}}`,
		`{"type":"buzzer.test","params":{"duration_ms":1000}}`,
	} {
		code, r := commandHTTPCall(t, f, "POST", "/api/v1/devices/car-001/commands", body)
		if code != 202 || r.Code != 0 {
			t.Fatal(code, r)
		}
		data := r.Data.(map[string]any)
		id := data["command_id"].(string)
		if ids[id] || data["status"] != "pending" {
			t.Fatal("bad acceptance", data)
		}
		ids[id] = true
		view := viewHTTP(t, f, id, "success")
		if view.AckAt == nil || view.ExecutedAt == nil || string(view.Error) != "null" || len(view.Result) == 0 || view.ExpiresAt.Sub(view.IssuedAt) != 10*time.Second {
			t.Fatal(view)
		}
		for _, c := range clients {
			ws := commandWSEvent(t, c, id, "success")
			if ws.Type != view.Type || !equalJSON(t, ws.Params, view.Params) || !equalJSON(t, ws.Result, view.Result) {
				t.Fatal("HTTP/WS mismatch")
			}
		}
	}
	if f.publisher.executor.Executions() != 5 {
		t.Fatal("wrong simulated execution count")
	}
	if err := f.monitor.Ingest(context.Background(), "car/car-001/telemetry", testutil.JSON(testutil.Telemetry("car-001", time.Now().UTC(), 1))); err != nil {
		t.Fatal(err)
	}
	code, r := commandHTTPCall(t, f, "GET", "/api/v1/devices/car-001/state", "")
	if code != 200 || r.Code != 0 {
		t.Fatal("monitoring regressed", r)
	}
}

func TestCommandsHTTPRejectsWithoutRecords(t *testing.T) {
	f := commandHTTP(t, "success")
	for _, body := range []string{`null`, `{"type":"camera.snapshot","params":{}}`, `{"type":"buzzer.test","params":{"duration_ms":99}}`, `{"type":"scenario.stop","params":{},"extra":1}`, `{"type":"scenario.stop","params":{}} {}`, strings.Repeat(" ", 65<<10) + `{}`} {
		code, r := commandHTTPCall(t, f, "POST", "/api/v1/devices/car-001/commands", body)
		if code != 400 || r.Code != 40001 {
			t.Fatal(code, r)
		}
	}
	body := `{"type":"scenario.stop","params":{}}`
	code, r := commandHTTPCall(t, f, "POST", "/api/v1/devices/missing/commands", body)
	if code != 404 || r.Code != 40401 {
		t.Fatal(code, r)
	}
	code, r = commandHTTPCall(t, f, "GET", "/api/v1/commands/missing", "")
	if code != 404 || r.Code != 40402 {
		t.Fatal(code, r)
	}
	f.publisher.available.Store(false)
	code, r = commandHTTPCall(t, f, "POST", "/api/v1/devices/car-001/commands", body)
	if code != 503 || r.Code != 50302 {
		t.Fatal(code, r)
	}
	if err := f.monitor.Ingest(context.Background(), "car/car-001/status", testutil.JSON(testutil.Status("car-001", time.Now().UTC(), false))); err != nil {
		t.Fatal(err)
	}
	code, r = commandHTTPCall(t, f, "POST", "/api/v1/devices/car-001/commands", body)
	if code != 503 || r.Code != 50301 {
		t.Fatal(code, r)
	}
	var count int64
	if err := f.database.Model(&model.Command{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("invalid requests inserted records", count, err)
	}
}

func TestCommandsHTTPSimulatedFailureAndTimeout(t *testing.T) {
	for _, mode := range []string{"failed", "none"} {
		t.Run(mode, func(t *testing.T) {
			f := commandHTTP(t, mode)
			ws := commandWS(t, f)
			code, r := commandHTTPCall(t, f, "POST", "/api/v1/devices/car-001/commands", `{"type":"scenario.stop","params":{}}`)
			if code != 202 {
				t.Fatal(code, r)
			}
			id := r.Data.(map[string]any)["command_id"].(string)
			status := "failed"
			if mode == "none" {
				viewHTTP(t, f, id, "sent")
				if err := f.database.Model(&model.Command{}).Where("command_id = ?", id).Update("expires_at", time.Now().UTC().Add(-time.Second)).Error; err != nil {
					t.Fatal(err)
				}
				status = "timeout"
			}
			view := viewHTTP(t, f, id, status)
			if mode == "failed" {
				if view.AckAt == nil || !strings.Contains(string(view.Error), "ACTUATOR_UNAVAILABLE") {
					t.Fatal(view)
				}
			} else if view.AckAt != nil || view.ExecutedAt != nil {
				t.Fatal("timeout invented Ack")
			}
			commandWSEvent(t, ws, id, status)
		})
	}
}
