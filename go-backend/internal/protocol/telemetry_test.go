package protocol_test

import (
	"encoding/json"
	"piguard/go-backend/internal/protocol"
	"piguard/go-backend/internal/testutil"
	"testing"
	"time"
)

func TestProtocolValidationAndViews(t *testing.T) {
	at := time.Date(2026, 9, 30, 8, 0, 0, 0, time.FixedZone("offset", 8*3600))
	msg := testutil.Telemetry("car-001", at, 0)
	decoded, err := protocol.ParseTelemetry("car/car-001/telemetry", testutil.JSON(msg))
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Timestamp.Location() != time.UTC || decoded.Speed.SampleAt.Location() != time.UTC {
		t.Fatal("time is not normalized")
	}
	view := protocol.View("car-001", true, decoded, at)
	if *view.Distance != 18 || view.Freshness["distance"] != "fresh" {
		t.Fatalf("wrong mapping: %+v", view)
	}
	for name, mutate := range map[string]func(*protocol.Telemetry){
		"version":   func(m *protocol.Telemetry) { m.SchemaVersion = 2 },
		"id":        func(m *protocol.Telemetry) { m.DeviceID = "other" },
		"timestamp": func(m *protocol.Telemetry) { m.Timestamp = time.Time{} },
		"seq":       func(m *protocol.Telemetry) { m.Seq = nil },
		"status":    func(m *protocol.Telemetry) { m.Speed.Status = "healthy" },
		"unit":      func(m *protocol.Telemetry) { m.Distance.Unit = "cm" },
		"enum":      func(m *protocol.Telemetry) { m.Risk.Level = "good" },
	} {
		t.Run(name, func(t *testing.T) {
			m := testutil.Telemetry("car-001", at, 1)
			mutate(m)
			if _, err := protocol.ParseTelemetry("car/car-001/telemetry", testutil.JSON(m)); err == nil {
				t.Fatal("invalid message accepted")
			}
		})
	}
	for _, raw := range []string{"null", `{`, `{"schema_version":1}`} {
		if _, err := protocol.ParseTelemetry("car/car-001/telemetry", []byte(raw)); err == nil {
			t.Fatal("invalid JSON accepted")
		}
	}
	msg.Speed.Value = nil
	msg.Temperature.Status = "timeout"
	msg.Lane.Valid = false
	view = protocol.View("car-001", false, msg, at.Add(2*time.Second))
	if view.Speed != nil || view.Temperature != nil || view.Lane.OffsetRatio != nil || view.Freshness["speed"] != "missing" || view.Freshness["temperature"] != "fault" || view.Freshness["distance"] != "stale" {
		t.Fatalf("bad missing/fault/stale view: %+v", view)
	}
	msg = testutil.Telemetry("car-001", at, 1)
	if protocol.View("car-001", true, msg, at.Add(time.Second)).Freshness["speed"] != "fresh" {
		t.Fatal("TTL endpoint must be fresh")
	}
	if protocol.View("car-001", true, msg, at.Add(5*time.Second+time.Nanosecond)).Freshness["temperature"] != "stale" {
		t.Fatal("temperature TTL not applied")
	}
	empty := protocol.View("car-001", false, nil, at)
	raw := testutil.JSON(empty)
	var fields map[string]any
	_ = json.Unmarshal(raw, &fields)
	if fields["speed_kmh"] != nil || empty.RiskLevel != "unknown" || empty.Freshness["lane"] != "missing" {
		t.Fatal("empty state invented values")
	}
}

func TestStatusWillAndRequiredFields(t *testing.T) {
	for _, online := range []bool{false, true} {
		msg := testutil.Status("car-001", time.Now(), online)
		if _, err := protocol.ParseStatus("car/car-001/status", testutil.JSON(msg)); err != nil {
			t.Fatal(err)
		}
	}
	for _, raw := range []string{`{"schema_version":1,"device_id":"car-001"}`, `{"schema_version":1,"device_id":"car-001","online":true}`, `{"schema_version":2,"device_id":"car-001","online":false}`} {
		if _, err := protocol.ParseStatus("car/car-001/status", []byte(raw)); err == nil {
			t.Fatal("invalid status accepted")
		}
	}
}
