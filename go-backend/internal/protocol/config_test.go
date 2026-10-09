package protocol

import (
	"encoding/json"
	"errors"
	"piguard/go-backend/internal/apperr"
	"piguard/go-backend/internal/model"
	"strings"
	"testing"
	"time"
)

func TestConfigPartialMergeAndValidation(t *testing.T) {
	r, err := ParseConfigRequest([]byte(`{"expected_version":1,"rules":{"obstacle":{"warning_distance_m":20},"sharp_turn":{"min_speed_kmh":0},"temperature":{"trigger_c":-5,"recover_c":-7}}}`))
	if err != nil {
		t.Fatal(err)
	}
	merged, err := r.Merge(model.DefaultRules().Rules)
	if err != nil {
		t.Fatal(err)
	}
	if merged.Obstacle.WarningDistanceM != 20 || merged.Obstacle.DangerDistanceM != 8 || merged.SharpTurn.MinSpeedKmh != 0 || merged.LaneDeparture != model.DefaultRules().Rules.LaneDeparture || merged.Temperature.TriggerC != -5 {
		t.Fatal(merged)
	}
	for _, raw := range []string{
		`null`, `[]`, `{}`, `{"expected_version":null,"rules":{"obstacle":{"warning_distance_m":20}}}`,
		`{"expected_version":0,"rules":{"obstacle":{"warning_distance_m":20}}}`,
		`{"expected_version":1.5,"rules":{"obstacle":{"warning_distance_m":20}}}`,
		`{"expected_version":1,"rules":null}`, `{"expected_version":1,"rules":{}}`,
		`{"expected_version":1,"rules":{"obstacle":{}}}`, `{"expected_version":1,"rules":{"obstacle":{"warning_distance_m":null}}}`,
		`{"expected_version":1,"rules":{"obstacle":{"warning_distance_m":"20"}}}`,
		`{"expected_version":1,"rules":{"lane_departure":{"duration_ms":1.2}}}`,
		`{"expected_version":1,"rules":{"lane_departure":{"duration_ms":0}}}`,
		`{"expected_version":1,"rules":{"camera":{}}}`,
		`{"expected_version":1,"rules":{"obstacle":{"extra":1}}}`,
		`{"expected_version":1,"rules":{"obstacle":{"warning_distance_m":1e999}}}`,
		`{"expected_version":1,"rules":{"obstacle":{"warning_distance_m":20}},"extra":true}`,
		`{"expected_version":1,"rules":{"obstacle":{"warning_distance_m":20}}} {}`,
	} {
		t.Run(raw, func(t *testing.T) {
			if _, err := ParseConfigRequest([]byte(raw)); err == nil {
				t.Fatal("accepted invalid request")
			}
		})
	}
	for _, patch := range []string{`{"obstacle":{"danger_distance_m":15}}`, `{"obstacle":{"warning_distance_m":0}}`, `{"lane_departure":{"offset_threshold":1.1}}`, `{"sharp_turn":{"yaw_threshold_deg_s":0}}`, `{"sharp_turn":{"min_speed_kmh":-1}}`, `{"temperature":{"recover_c":35}}`} {
		r := ConfigRequest{ExpectedVersion: 1, Rules: json.RawMessage(patch)}
		_, err := r.Merge(model.DefaultRules().Rules)
		var invalid *apperr.InvalidParams
		if !errors.As(err, &invalid) || !strings.HasPrefix(invalid.Field, "rules.") {
			t.Fatal(patch, err)
		}
	}
}

func TestConfigMessageAndAckIdentityAndStructure(t *testing.T) {
	m := ConfigMessage{SchemaVersion: 1, DeviceID: "car-001", ConfigVersion: 2, IssuedAt: time.Now().UTC(), Rules: model.DefaultRules().Rules}
	raw, _ := json.Marshal(m)
	if _, err := ParseConfigMessage("car/car-001/config", raw); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseConfigMessage("car/car-002/config", raw); err == nil {
		t.Fatal("accepted wrong identity")
	}
	if _, err := ParseConfigMessage("car/car-001/config", []byte(`{"schema_version":1,"device_id":"car-001","config_version":2,"issued_at":"2026-10-09T00:00:00Z","rules":{"obstacle":{"warning_distance_m":20}}}`)); err == nil {
		t.Fatal("accepted incomplete wire rules")
	}
	good := `{"schema_version":1,"device_id":"car-001","config_version":2,"status":"success","applied_at":"2026-10-09T08:00:00+08:00"}`
	a, err := ParseConfigAck("car/car-001/config-acks", []byte(good))
	if err != nil || a.AppliedAt.Format(time.RFC3339) != "2026-10-09T00:00:00Z" {
		t.Fatal(a, err)
	}
	for _, raw := range []string{strings.Replace(good, `"schema_version":1`, `"schema_version":2`, 1), strings.Replace(good, `"config_version":2`, `"config_version":0`, 1), strings.Replace(good, `"car-001"`, `"car-002"`, 1), strings.Replace(good, `"success"`, `"sent"`, 1), strings.Replace(good, `"2026-10-09T08:00:00+08:00"`, `"2026-10-09"`, 1), `{"schema_version":1,"device_id":"car-001","config_version":2,"status":"success"}`, `{"schema_version":1,"device_id":"car-001","config_version":2,"status":"failed","error":{"code":"","message":"bad"}}`, `{"schema_version":1,"device_id":"car-001","config_version":2,"status":"failed","error":{"code":"BAD","message":"bad","extra":1}}`, strings.TrimSuffix(good, "}") + `,"error":null}`} {
		if _, err := ParseConfigAck("car/car-001/config-acks", []byte(raw)); err == nil {
			t.Fatal("accepted invalid Ack", raw)
		}
	}
	if _, err := ParseConfigAck("car/car-001/config-acks", []byte(`{"schema_version":1,"device_id":"car-001","config_version":2,"status":"failed","error":{"code":"BAD","message":"bad"}}`)); err != nil {
		t.Fatal(err)
	}
}
