package protocol

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func eventBody(mutate func(map[string]any)) []byte {
	body := map[string]any{
		"schema_version": 1,
		"event_id":       "evt-001",
		"device_id":      "car-001",
		"type":           "obstacle_warning",
		"action":         "started",
		"level":          "danger",
		"timestamp":      "2026-10-08T20:00:00+08:00",
		"data":           map[string]any{"distance_m": 6.8},
	}
	if mutate != nil {
		mutate(body)
	}
	raw, err := json.Marshal(body)
	if err != nil {
		panic(err)
	}
	return raw
}

func TestParseEventAcceptsLifecycleAndOptionalSnapshot(t *testing.T) {
	for _, item := range []struct {
		typ, action, level string
	}{
		{"obstacle_warning", "started", "warning"},
		{"lane_departure", "level_changed", "danger"},
		{"sharp_turn", "recovered", "warning"},
		{"high_temperature", "started", "danger"},
		{"sensor_failure", "recovered", "danger"},
	} {
		raw := eventBody(func(m map[string]any) {
			m["type"], m["action"], m["level"] = item.typ, item.action, item.level
			m["data"] = map[string]any{}
			m["snapshot_available"] = true
			m["snapshot_id"] = "snap-001"
		})
		event, err := ParseEvent("car/car-001/events", raw)
		if err != nil {
			t.Fatal(item, err)
		}
		if !event.Timestamp.Equal(time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)) || event.SnapshotID == nil || *event.SnapshotID != "snap-001" || event.SnapshotAvailable == nil || !*event.SnapshotAvailable {
			t.Fatal(event)
		}
	}
	raw := eventBody(nil)
	first, err := ParseEvent("car/car-001/events", raw)
	if err != nil {
		t.Fatal(err)
	}
	replay := *first
	replay.SnapshotAvailable = nil
	if !replay.SameRecord(first.DeviceID, first.Type, first.Action, first.Level, first.Timestamp, string(first.Data), "") {
		t.Fatal("snapshot flag changed identity")
	}
}

func TestParseEventRejectsInvalidMessages(t *testing.T) {
	cases := []struct {
		topic  string
		mutate func(map[string]any)
		raw    string
	}{
		{topic: "car/car-002/events"},
		{mutate: func(m map[string]any) { m["schema_version"] = 2 }},
		{mutate: func(m map[string]any) { m["event_id"] = "" }},
		{mutate: func(m map[string]any) { m["event_id"] = "evt 1" }},
		{mutate: func(m map[string]any) { m["event_id"] = strings.Repeat("a", 65) }},
		{mutate: func(m map[string]any) { m["type"] = "updated" }},
		{mutate: func(m map[string]any) { m["action"] = "updated" }},
		{mutate: func(m map[string]any) { m["level"] = "normal" }},
		{mutate: func(m map[string]any) { m["timestamp"] = "2026-10-08T12:00:00" }},
		{mutate: func(m map[string]any) { m["data"] = []any{1} }},
		{mutate: func(m map[string]any) { m["data"] = nil }},
		{mutate: func(m map[string]any) { delete(m, "data") }},
		{mutate: func(m map[string]any) { m["snapshot_id"] = nil }},
		{mutate: func(m map[string]any) { m["snapshot_id"] = "" }},
		{mutate: func(m map[string]any) { m["snapshot_available"] = "true" }},
		{mutate: func(m map[string]any) { m["snapshot_available"] = nil }},
		{mutate: func(m map[string]any) { m["message_id"] = "extra" }},
		{raw: `{"schema_version":1}{"schema_version":1}`},
		{raw: `[]`},
		{raw: `not-json`},
	}
	for _, tc := range cases {
		raw := []byte(tc.raw)
		if tc.raw == "" {
			raw = eventBody(tc.mutate)
		}
		topic := tc.topic
		if topic == "" {
			topic = "car/car-001/events"
		}
		if _, err := ParseEvent(topic, raw); err == nil {
			t.Fatalf("accepted %s %s", topic, raw)
		}
	}
}
