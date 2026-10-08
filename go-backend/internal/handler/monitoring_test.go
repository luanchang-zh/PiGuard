package handler

import (
	"context"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"net/http/httptest"
	"path/filepath"
	"piguard/go-backend/internal/db"
	"piguard/go-backend/internal/model"
	"piguard/go-backend/internal/realtime"
	"piguard/go-backend/internal/repo"
	"piguard/go-backend/internal/service"
	"piguard/go-backend/internal/testutil"
	"strings"
	"testing"
	"time"
)

func monitoringEngine(t *testing.T) (*gin.Engine, *service.Monitor, *realtime.Hub, repo.TelemetryRepository) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	database, err := db.Open(filepath.Join(t.TempDir(), "monitor.db"))
	if err != nil {
		t.Fatal(err)
	}
	sql, _ := database.DB()
	t.Cleanup(func() { _ = sql.Close() })
	d, h := repo.NewDeviceRepository(database), repo.NewTelemetryRepository(database)
	hub := realtime.NewHub(32)
	t.Cleanup(hub.Close)
	m := service.NewMonitor(d, h, hub)
	svc := service.NewDeviceService(d, repo.NewConfigRepository(database), m)
	if _, err := svc.Seed(context.Background(), service.SeedInput{DeviceID: "car-001", Name: "Demo Car", Mode: "mock", SoftwareVersion: "0.1.0"}); err != nil {
		t.Fatal(err)
	}
	return NewEngine(Dependencies{Ping: sql.PingContext, Devices: svc, Telemetry: service.NewTelemetryService(h, d), Hub: hub}), m, hub, h
}

func decode(t *testing.T, body string) map[string]any {
	t.Helper()
	var result map[string]any
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestMonitoringHTTP(t *testing.T) {
	engine, m, _, history := monitoringEngine(t)
	ctx := context.Background()
	at := time.Now().UTC().Truncate(time.Second)
	for _, path := range []string{"/api/v1/devices", "/api/v1/devices/car-001", "/api/v1/devices/car-001/state", "/api/v1/devices/car-001/telemetry"} {
		status, body := getJSON(engine, path)
		if status != 200 || decode(t, body)["code"] != float64(0) {
			t.Fatalf("%s: %d %s", path, status, body)
		}
	}
	_, body := getJSON(engine, "/api/v1/devices/car-001/state")
	state := decode(t, body)["data"].(map[string]any)
	if state["speed_kmh"] != nil || state["risk_level"] != "unknown" {
		t.Fatal(body)
	}
	if err := m.Ingest(ctx, "car/car-001/status", testutil.JSON(testutil.Status("car-001", at, true))); err != nil {
		t.Fatal(err)
	}
	if err := m.Ingest(ctx, "car/car-001/telemetry", testutil.JSON(testutil.Telemetry("car-001", at, 1))); err != nil {
		t.Fatal(err)
	}
	_, body = getJSON(engine, "/api/v1/devices/car-001")
	device := decode(t, body)["data"].(map[string]any)
	if device["online"] != true || device["config_version"] != float64(5) || device["sensors"] == nil || device["ID"] != nil {
		t.Fatal(body)
	}
	for i := 1; i <= 4; i++ {
		row := &model.Telemetry{DeviceID: "car-001", Timestamp: at.Add(time.Duration(i) * time.Second), Seq: uint64(i), Speed: testutil.Ptr(float64(i)), RiskLevel: "normal"}
		if err := history.Insert(ctx, row); err != nil {
			t.Fatal(err)
		}
	}
	_, body = getJSON(engine, "/api/v1/devices/car-001/telemetry?limit=2")
	items := decode(t, body)["data"].(map[string]any)["items"].([]any)
	if len(items) != 2 || items[0].(map[string]any)["speed_kmh"] != float64(3) || items[1].(map[string]any)["speed_kmh"] != float64(4) {
		t.Fatal(body)
	}
	path := "/api/v1/devices/car-001/telemetry?start=" + at.Add(2*time.Second).Format(time.RFC3339) + "&end=" + at.Add(3*time.Second).Format(time.RFC3339)
	_, body = getJSON(engine, path)
	if len(decode(t, body)["data"].(map[string]any)["items"].([]any)) != 2 {
		t.Fatal(body)
	}
	for _, suffix := range []string{"", "/state", "/telemetry"} {
		status, body := getJSON(engine, "/api/v1/devices/missing"+suffix)
		if status != 404 || decode(t, body)["code"] != float64(40401) {
			t.Fatal(status, body)
		}
	}
	for _, query := range []string{"limit=0", "limit=1001", "limit=bad", "limit=", "start=bad", "end=", "start=2026-09-30T08:00:00Z&end=2026-09-29T08:00:00Z"} {
		status, body := getJSON(engine, "/api/v1/devices/car-001/telemetry?"+query)
		if status != 400 || decode(t, body)["code"] != float64(40001) || decode(t, body)["error"] == nil {
			t.Fatal(status, body)
		}
	}
}

func TestTwoWebsocketClientsAndShutdown(t *testing.T) {
	engine, m, hub, _ := monitoringEngine(t)
	server := httptest.NewServer(engine)
	defer server.Close()
	url := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/v1/ws"
	a, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	if err := m.Ingest(context.Background(), "car/car-001/status", testutil.JSON(testutil.Status("car-001", time.Now(), true))); err != nil {
		t.Fatal(err)
	}
	if err := m.Ingest(context.Background(), "car/car-001/telemetry", testutil.JSON(testutil.Telemetry("car-001", time.Now(), 1))); err != nil {
		t.Fatal(err)
	}
	for _, conn := range []*websocket.Conn{a, b} {
		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		for _, kind := range []string{"device_status", "telemetry"} {
			var event realtime.Event
			if err := conn.ReadJSON(&event); err != nil || event.Type != kind {
				t.Fatalf("wrong WS event %s: %+v %v", kind, event, err)
			}
		}
	}
	_ = a.Close()
	if err := m.Ingest(context.Background(), "car/car-001/status", testutil.JSON(testutil.Status("car-001", time.Now(), false))); err != nil {
		t.Fatal(err)
	}
	var event realtime.Event
	if err := b.ReadJSON(&event); err != nil || event.Type != "device_status" {
		t.Fatal(event, err)
	}
	// A pending telemetry update may precede the close; drain until EOF.
	hub.Close()
	_ = b.SetReadDeadline(time.Now().Add(2 * time.Second))
	for {
		if err := b.ReadJSON(&event); err != nil {
			break
		}
	}
}
