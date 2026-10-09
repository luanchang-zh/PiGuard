package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"piguard/go-backend/internal/db"
	"piguard/go-backend/internal/protocol"
	"piguard/go-backend/internal/realtime"
	"piguard/go-backend/internal/repo"
	"piguard/go-backend/internal/service"
	"piguard/go-backend/internal/simulator"
)

type configHTTPPublisher struct {
	available atomic.Bool
	executor  *simulator.ConfigExecutor
	ingest    func(context.Context, string, []byte) error
	sent      chan protocol.ConfigMessage
}

func (p *configHTTPPublisher) Available() bool { return p.available.Load() }
func (p *configHTTPPublisher) Publish(ctx context.Context, topic string, qos byte, retained bool, raw []byte) error {
	if qos != 1 || !retained {
		return errors.New("config must be QoS 1 retained")
	}
	message, err := protocol.ParseConfigMessage(topic, raw)
	if err != nil {
		return err
	}
	p.sent <- *message
	ack, _, err := p.executor.Apply(topic, raw, time.Now().UTC())
	if err != nil {
		return err
	}
	if ack != nil {
		encoded, _ := json.Marshal(ack)
		return p.ingest(ctx, "car/"+message.DeviceID+"/config-acks", encoded)
	}
	return nil
}
func configHTTPFixture(t *testing.T, mode string) (*httptest.Server, *service.ConfigManager, *configHTTPPublisher, repo.DeviceRepository) {
	t.Helper()
	g, err := db.Open(filepath.Join(t.TempDir(), "config-http.db"))
	if err != nil {
		t.Fatal(err)
	}
	sql, _ := g.DB()
	t.Cleanup(func() { sql.Close() })
	r := repo.NewConfigRepository(g)
	d := repo.NewDeviceRepository(g)
	devices := service.NewDeviceService(d, r)
	if _, err = devices.Seed(context.Background(), service.SeedInput{DeviceID: "car-001", Name: "Demo", Mode: "mock", SoftwareVersion: "0.1.0"}); err != nil {
		t.Fatal(err)
	}
	executor, err := simulator.NewConfigExecutor(mode)
	if err != nil {
		t.Fatal(err)
	}
	p := &configHTTPPublisher{executor: executor, sent: make(chan protocol.ConfigMessage, 32)}
	p.available.Store(true)
	s := service.NewConfigService(r, p, d)
	p.ingest = s.IngestAck
	if err = s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	hub := realtime.NewHub(32)
	t.Cleanup(hub.Close)
	server := httptest.NewServer(NewEngine(Dependencies{Configs: s, Devices: devices, Hub: hub}))
	t.Cleanup(server.Close)
	return server, s, p, d
}
func configHTTPCall(t *testing.T, server *httptest.Server, method, path, body string) (int, map[string]any) {
	t.Helper()
	request, err := http.NewRequest(method, server.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var result map[string]any
	if err = json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	return response.StatusCode, result
}
func configHTTPData(t *testing.T, s *httptest.Server, status string) map[string]any {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		code, result := configHTTPCall(t, s, "GET", "/api/v1/devices/car-001/config", "")
		if code != 200 || result["code"] != float64(0) {
			t.Fatal(result)
		}
		data := result["data"].(map[string]any)
		if data["status"] == status {
			return data
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("config status not reached", status)
	return nil
}
func TestConfigHTTPSoftwareDeviceRoundTripAndNoNewWS(t *testing.T) {
	for _, mode := range []string{"success", "failed", "none"} {
		t.Run(mode, func(t *testing.T) {
			server, _, p, d := configHTTPFixture(t, mode)
			first := configHTTPData(t, server, "pending")
			if first["desired_version"] != float64(1) || first["reported_version"] != float64(0) || first["ack_at"] != nil || first["error"] != nil {
				t.Fatal(first)
			}
			wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/v1/ws"
			a, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer a.Close()
			b, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer b.Close()
			code, result := configHTTPCall(t, server, "PUT", "/api/v1/devices/car-001/config", `{"expected_version":1,"rules":{"obstacle":{"warning_distance_m":20}}}`)
			if code != 202 || result["code"] != float64(0) || result["data"].(map[string]any)["status"] != "pending" || result["data"].(map[string]any)["config_version"] != float64(2) {
				t.Fatal(code, result)
			}
			select {
			case message := <-p.sent:
				if message.ConfigVersion != 2 || message.Rules.Obstacle.DangerDistanceM != 8 || message.Rules.Obstacle.WarningDistanceM != 20 || message.IssuedAt.IsZero() {
					t.Fatal(message)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("device did not receive config")
			}
			want := mode
			if mode == "none" {
				want = "pending"
			}
			data := configHTTPData(t, server, want)
			if data["desired_version"] != float64(2) || data["ID"] != nil {
				t.Fatal(data)
			}
			if mode == "success" && (data["reported_version"] != float64(2) || data["ack_at"] == nil || data["applied_at"] == nil || data["error"] != nil) {
				t.Fatal(data)
			}
			if mode == "failed" && (data["reported_version"] != float64(0) || data["ack_at"] == nil || data["applied_at"] != nil || data["error"].(map[string]any)["code"] != "INVALID_CONFIG") {
				t.Fatal(data)
			}
			if mode == "none" && (data["reported_version"] != float64(0) || data["ack_at"] != nil) {
				t.Fatal(data)
			}
			// Config results do not alter device status report facts.
			if err = d.Update(context.Background(), "car-001", map[string]any{"reported_config_version": 99}); err != nil {
				t.Fatal(err)
			}
			_, device := configHTTPCall(t, server, "GET", "/api/v1/devices/car-001", "")
			if device["data"].(map[string]any)["config_version"] != float64(99) {
				t.Fatal(device)
			}
			after := configHTTPData(t, server, want)
			if after["reported_version"] != data["reported_version"] {
				t.Fatal("status forged config Ack")
			}
			for _, conn := range []*websocket.Conn{a, b} {
				conn.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
				if _, raw, err := conn.ReadMessage(); err == nil {
					t.Fatal("unspecified config WS broadcast", string(raw))
				}
			}
		})
	}
}
func TestConfigHTTPValidationConflictAndUnavailable(t *testing.T) {
	server, _, p, _ := configHTTPFixture(t, "none")
	for _, tc := range []struct{ body, field string }{
		{`{}`, "expected_version"}, {`{"rules":{"obstacle":{"warning_distance_m":20}}}`, "expected_version"},
		{`{"expected_version":1,"rules":{}}`, "rules"},
		{`{"expected_version":1,"rules":{"obstacle":{"danger_distance_m":20}}}`, "rules.obstacle.danger_distance_m"},
		{`{"expected_version":1,"rules":{"obstacle":{"warning_distance_m":null}}}`, "rules.obstacle.warning_distance_m"},
		{`{"expected_version":1,"rules":{"extra":{}}}`, "rules.extra"},
	} {
		code, result := configHTTPCall(t, server, "PUT", "/api/v1/devices/car-001/config", tc.body)
		if code != 400 || result["code"] != float64(40001) || result["error"].(map[string]any)["field"] != tc.field {
			t.Fatal(tc, code, result)
		}
	}
	body := `{"expected_version":1,"rules":{"obstacle":{"warning_distance_m":20}}}`
	for _, method := range []string{"GET", "PUT"} {
		code, result := configHTTPCall(t, server, method, "/api/v1/devices/unknown/config", body)
		if code != 404 || result["code"] != float64(40401) {
			t.Fatal(code, result)
		}
	}
	p.available.Store(false)
	code, result := configHTTPCall(t, server, "PUT", "/api/v1/devices/car-001/config", body)
	if code != 503 || result["code"] != float64(50302) {
		t.Fatal(code, result)
	}
	if configHTTPData(t, server, "pending")["desired_version"] != float64(1) {
		t.Fatal("rejected updates mutated version")
	}
	p.available.Store(true)
	code, _ = configHTTPCall(t, server, "PUT", "/api/v1/devices/car-001/config", body)
	if code != 202 {
		t.Fatal(code)
	}
	code, result = configHTTPCall(t, server, "PUT", "/api/v1/devices/car-001/config", body)
	if code != 409 || result["code"] != float64(40902) {
		t.Fatal(code, result)
	}
	// Body size limit returns the same field-aware rejection envelope.
	req, _ := http.NewRequest("PUT", server.URL+"/api/v1/devices/car-001/config", bytes.NewReader(bytes.Repeat([]byte("x"), 65<<10)))
	response, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	io.Copy(io.Discard, response.Body)
	if response.StatusCode != 400 {
		t.Fatal(response.StatusCode)
	}
}
