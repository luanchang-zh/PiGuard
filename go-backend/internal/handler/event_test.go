package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
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
)

type eventFixture struct {
	path    string
	server  *httptest.Server
	db      *gorm.DB
	hub     *realtime.Hub
	events  service.EventService
	devices repo.DeviceRepository
}

func newEventFixture(t *testing.T) *eventFixture {
	t.Helper()
	f := &eventFixture{path: filepath.Join(t.TempDir(), "events.db")}
	t.Cleanup(f.close)
	f.open(t)
	return f
}

func (f *eventFixture) open(t *testing.T) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	g, err := db.Open(f.path)
	if err != nil {
		t.Fatal(err)
	}
	f.db = g
	f.devices = repo.NewDeviceRepository(g)
	f.hub = realtime.NewHub(32)
	monitor := service.NewMonitor(f.devices, repo.NewTelemetryRepository(g), f.hub)
	devices := service.NewDeviceService(f.devices, repo.NewConfigRepository(g), monitor)
	for _, id := range []string{"car-001", "car-002"} {
		if _, err := f.devices.Find(context.Background(), id); err == nil {
			continue
		}
		if _, err := devices.Seed(context.Background(), service.SeedInput{DeviceID: id, Name: id, Mode: "mock", SoftwareVersion: "0.1.0"}); err != nil {
			t.Fatal(err)
		}
	}
	f.events = service.NewEventService(repo.NewEventRepository(g), f.devices, f.hub)
	sql, _ := g.DB()
	f.server = httptest.NewServer(NewEngine(Dependencies{
		Ping: sql.PingContext, Devices: devices, Telemetry: service.NewTelemetryService(repo.NewTelemetryRepository(g), f.devices),
		Events: f.events, Hub: f.hub, Frames: service.NewFrameService(repo.NewSnapshotRepository(g), t.TempDir()),
	}))
}

func (f *eventFixture) close() {
	if f.server != nil {
		f.server.Close()
		f.server = nil
	}
	if f.hub != nil {
		f.hub.Close()
		f.hub = nil
	}
	if f.db != nil {
		sql, _ := f.db.DB()
		_ = sql.Close()
		f.db = nil
	}
}

func (f *eventFixture) call(t *testing.T, path string) (int, Response, []byte) {
	t.Helper()
	response, err := http.Get(f.server.URL + path)
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
	return response.StatusCode, envelope, raw
}

func (f *eventFixture) items(t *testing.T, path string) []protocol.EventItem {
	t.Helper()
	code, envelope, raw := f.call(t, path)
	if code != 200 || envelope.Code != 0 {
		t.Fatal(code, string(raw))
	}
	encoded, _ := json.Marshal(envelope.Data)
	var body struct {
		Items []protocol.EventItem `json:"items"`
	}
	if err := json.Unmarshal(encoded, &body); err != nil {
		t.Fatal(err)
	}
	return body.Items
}

func alarmRaw(id, device, typ, action, level, at, data string, extra map[string]any) []byte {
	body := map[string]any{
		"schema_version": 1, "event_id": id, "device_id": device, "type": typ,
		"action": action, "level": level, "timestamp": at, "data": json.RawMessage(data),
	}
	for key, value := range extra {
		body[key] = value
	}
	raw, err := json.Marshal(body)
	if err != nil {
		panic(err)
	}
	return raw
}

func readPush(t *testing.T, c *websocket.Conn) protocol.EventPush {
	t.Helper()
	_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
	for {
		var env struct {
			Type      string             `json:"type"`
			Timestamp time.Time          `json:"timestamp"`
			Data      protocol.EventPush `json:"data"`
		}
		if err := c.ReadJSON(&env); err != nil {
			t.Fatal(err)
		}
		if env.Type != "event" {
			continue
		}
		if env.Timestamp.IsZero() || env.Data.EventID == "" {
			t.Fatal(env)
		}
		return env.Data
	}
}

func TestAlarmEventsHTTPAndWebSocket(t *testing.T) {
	f := newEventFixture(t)
	if items := f.items(t, "/api/v1/devices/car-001/events"); len(items) != 0 {
		t.Fatal(items)
	}
	clients := make([]*websocket.Conn, 2)
	for i := range clients {
		c, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(f.server.URL, "http")+"/api/v1/ws", nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { c.Close() })
		clients[i] = c
	}
	base := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	messages := [][]byte{
		alarmRaw("evt-old", "car-001", "obstacle_warning", "started", "warning", base.Format(time.RFC3339), `{"distance_m":6.8}`, map[string]any{"snapshot_available": true}),
		alarmRaw("evt-lane", "car-001", "lane_departure", "level_changed", "danger", base.Add(time.Second).Format(time.RFC3339), `{"offset_ratio":0.4}`, nil),
		alarmRaw("evt-turn", "car-001", "sharp_turn", "recovered", "warning", base.Add(2*time.Second).Format(time.RFC3339), `{}`, nil),
		alarmRaw("evt-temp", "car-001", "high_temperature", "started", "danger", base.Add(3*time.Second).Format(time.RFC3339), `{"temperature_c":36}`, map[string]any{"snapshot_id": "snap-001"}),
		alarmRaw("evt-sensor", "car-001", "sensor_failure", "started", "warning", base.Add(4*time.Second).Format(time.RFC3339), `{"sensor":"distance","reason":"timeout"}`, nil),
	}
	for _, raw := range messages {
		if err := f.events.Ingest(context.Background(), "car/car-001/events", raw); err != nil {
			t.Fatal(err)
		}
	}
	early := alarmRaw("evt-earlier", "car-001", "obstacle_warning", "recovered", "danger", base.Add(-time.Minute).Format(time.RFC3339), `{"distance_m":20}`, nil)
	if err := f.events.Ingest(context.Background(), "car/car-001/events", early); err != nil {
		t.Fatal(err)
	}
	for _, c := range clients {
		seen := map[string]protocol.EventPush{}
		for range 6 {
			item := readPush(t, c)
			seen[item.EventID] = item
		}
		if seen["evt-temp"].EventType != "high_temperature" || seen["evt-temp"].SnapshotID == nil || *seen["evt-temp"].SnapshotID != "snap-001" {
			t.Fatal(seen["evt-temp"])
		}
		if seen["evt-old"].SnapshotID != nil || seen["evt-lane"].Action != "level_changed" {
			t.Fatal(seen)
		}
	}
	if err := f.events.Ingest(context.Background(), "car/car-001/events", messages[0]); err != nil {
		t.Fatal(err)
	}
	conflict := alarmRaw("evt-old", "car-001", "obstacle_warning", "started", "danger", base.Format(time.RFC3339), `{"distance_m":1}`, nil)
	if err := f.events.Ingest(context.Background(), "car/car-001/events", conflict); err != nil {
		t.Fatal(err)
	}
	if err := f.events.Ingest(context.Background(), "car/car-002/events", alarmRaw("evt-old", "car-002", "obstacle_warning", "started", "warning", base.Format(time.RFC3339), `{"distance_m":6.8}`, nil)); err != nil {
		t.Fatal(err)
	}
	for _, raw := range [][]byte{
		[]byte(`not-json`),
		alarmRaw("evt-bad", "car-001", "updated", "started", "warning", base.Format(time.RFC3339), `{}`, nil),
		alarmRaw("evt-bad", "car-009", "obstacle_warning", "started", "warning", base.Format(time.RFC3339), `{}`, nil),
		[]byte(`{"schema_version":1,"event_id":"evt-x","device_id":"car-001","type":"obstacle_warning","action":"started","level":"warning","timestamp":"2026-10-08T12:00:00Z","data":{},"extra":1}`),
	} {
		if err := f.events.Ingest(context.Background(), "car/car-001/events", raw); err == nil {
			t.Fatal("accepted", string(raw))
		}
	}
	for _, c := range clients {
		_ = c.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
		var extra protocol.EventPush
		if err := c.ReadJSON(&extra); err == nil {
			t.Fatal("unexpected push", extra)
		}
	}
	items := f.items(t, "/api/v1/devices/car-001/events?type=obstacle_warning&level=warning&start=2026-10-08T12:00:00Z&end=2026-10-08T12:00:00Z&unused=1")
	if len(items) != 1 || items[0].EventID != "evt-old" || items[0].Type != "obstacle_warning" || items[0].Level != "warning" || string(items[0].Data) == "" {
		t.Fatal(items)
	}
	var distance struct {
		Distance float64 `json:"distance_m"`
	}
	if err := json.Unmarshal(items[0].Data, &distance); err != nil || distance.Distance != 6.8 || items[0].SnapshotID != nil {
		t.Fatal(items[0], err)
	}
	listed := f.items(t, "/api/v1/devices/car-001/events")
	if len(listed) != 6 || listed[0].EventID != "evt-sensor" || listed[len(listed)-1].EventID != "evt-earlier" {
		t.Fatal(listed)
	}
	encoded, _ := json.Marshal(listed[0])
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &keys); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"event_id", "type", "action", "level", "timestamp", "data", "snapshot_id"} {
		if _, ok := keys[key]; !ok {
			t.Fatal("missing", key)
		}
	}
	if _, ok := keys["id"]; ok || strings.Contains(string(encoded), "snapshot_available") {
		t.Fatal(string(encoded))
	}
	if other := f.items(t, "/api/v1/devices/car-002/events"); len(other) != 0 {
		t.Fatal(other)
	}
	code, envelope, _ := f.call(t, "/api/v1/devices/missing/events")
	if code != 404 || envelope.Code != 40401 {
		t.Fatal(code, envelope)
	}
	for _, path := range []string{"?type=updated", "?level=normal", "?start=bad", "?end=bad", "?start=2026-10-08T12:00:02Z&end=2026-10-08T12:00:01Z", "?limit=0", "?limit=1001", "?limit=no"} {
		code, envelope, _ := f.call(t, "/api/v1/devices/car-001/events"+path)
		if code != 400 || envelope.Code != 40001 || envelope.Error == nil {
			t.Fatal(path, code, envelope)
		}
	}
	request, err := http.NewRequest(http.MethodPost, f.server.URL+"/api/v1/devices/car-001/frames", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNotImplemented {
		t.Fatal(response.StatusCode)
	}
	code, envelope, _ = f.call(t, "/api/v1/snapshots/snap-001/content")
	if code != http.StatusNotImplemented || envelope.Code != 50001 {
		t.Fatal(code, envelope)
	}
	var count int64
	if err := f.db.Model(&model.Event{}).Count(&count).Error; err != nil || count != 6 {
		t.Fatal(count, err)
	}
}

func TestAlarmEventRestartKeepsHistoryWithoutReplay(t *testing.T) {
	f := newEventFixture(t)
	at := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	raw := alarmRaw("evt-keep", "car-001", "high_temperature", "recovered", "danger", at.Format(time.RFC3339), `{"temperature_c":33}`, nil)
	if err := f.events.Ingest(context.Background(), "car/car-001/events", raw); err != nil {
		t.Fatal(err)
	}
	f.close()
	f.open(t)
	items := f.items(t, "/api/v1/devices/car-001/events")
	if len(items) != 1 || items[0].EventID != "evt-keep" || items[0].Action != "recovered" {
		t.Fatal(items)
	}
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(f.server.URL, "http")+"/api/v1/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	next := alarmRaw("evt-next", "car-001", "high_temperature", "started", "warning", at.Add(time.Second).Format(time.RFC3339), `{}`, nil)
	if err := f.events.Ingest(context.Background(), "car/car-001/events", next); err != nil {
		t.Fatal(err)
	}
	if got := readPush(t, conn); got.EventID != "evt-next" {
		t.Fatal(got)
	}
}

func TestAlarmEventDefaultLimit(t *testing.T) {
	f := newEventFixture(t)
	base := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 101; i++ {
		id := "evt-" + base.Add(time.Duration(i)*time.Second).Format("150405")
		raw := alarmRaw(id, "car-001", "lane_departure", "started", "warning", base.Add(time.Duration(i)*time.Second).Format(time.RFC3339), `{}`, nil)
		if err := f.events.Ingest(context.Background(), "car/car-001/events", raw); err != nil {
			t.Fatal(err)
		}
	}
	items := f.items(t, "/api/v1/devices/car-001/events")
	if len(items) != 100 || items[0].EventID != "evt-120140" || items[len(items)-1].EventID != "evt-120001" {
		t.Fatal(len(items), items[0].EventID, items[len(items)-1].EventID)
	}
	limited := f.items(t, "/api/v1/devices/car-001/events?limit=1000")
	if len(limited) != 101 {
		t.Fatal(len(limited))
	}
}
