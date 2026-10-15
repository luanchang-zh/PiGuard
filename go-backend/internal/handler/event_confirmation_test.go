package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

func TestDeviceEventConfirmationRequiresExactOwnedRecord(t *testing.T) {
	f := newAcceptanceFixture(t)
	raw := alarmRaw("confirm-owned", "car-001", "sensor_failure", "started", "warning", time.Now().UTC().Format(time.RFC3339Nano), `{"sensor":"gyro","reason":"error"}`, nil)
	if err := f.events.Ingest(context.Background(), "car/car-001/events", raw); err != nil {
		t.Fatal(err)
	}
	body := fmt.Sprintf(`{"events":[%s]}`, raw)
	path := "/api/v1/devices/car-001/events/confirmations"
	status, _, result := f.request(t, "POST", path, body, "car-001", "test-device", nil)
	if status != 200 {
		t.Fatalf("HTTP %d: %s", status, result)
	}
	var env struct {
		Data struct {
			IDs []string `json:"confirmed_event_ids"`
		} `json:"data"`
	}
	if err := json.Unmarshal(result, &env); err != nil || len(env.Data.IDs) != 1 || env.Data.IDs[0] != "confirm-owned" {
		t.Fatal(string(result), err)
	}
	for _, tc := range []struct {
		path, user, password string
		want                 int
	}{
		{path, "", "", 401},
		{path, "car-002", "test-device-2", 403},
		{"/api/v1/devices/car-002/events/confirmations", "car-001", "test-device", 403},
	} {
		status, _, _ := f.request(t, "POST", tc.path, body, tc.user, tc.password, nil)
		if status != tc.want {
			t.Fatalf("%s: got %d want %d", tc.path, status, tc.want)
		}
	}
	var event map[string]any
	if err := json.Unmarshal(raw, &event); err != nil {
		t.Fatal(err)
	}
	event["level"] = "danger"
	conflicting, _ := json.Marshal(map[string]any{"events": []any{event}})
	status, _, result = f.request(t, "POST", path, string(conflicting), "car-001", "test-device", nil)
	if status != 200 {
		t.Fatal(status, string(result))
	}
	if err := json.Unmarshal(result, &env); err != nil || len(env.Data.IDs) != 0 {
		t.Fatal("conflicting payload confirmed", string(result))
	}
	for _, bad := range []string{`{"events":[]}`, `{"events":[{}]}`, body + `{}`, `{"events":null}`, `{"events":[],"extra":1}`} {
		status, _, _ = f.request(t, "POST", path, bad, "car-001", "test-device", nil)
		if status != 400 {
			t.Fatalf("invalid body accepted: %d", status)
		}
	}
}
