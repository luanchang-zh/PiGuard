// Package protocol describes MQTT messages and public monitoring views.
// These types preserve sampling metadata independently of database models.
package protocol

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type Sample struct {
	Value    *float64   `json:"value"`
	Unit     string     `json:"unit"`
	Source   string     `json:"source"`
	Status   string     `json:"status"`
	SampleAt *time.Time `json:"sample_at"`
}

type Distance struct {
	RawValue    *float64   `json:"raw_value"`
	MappedValue *float64   `json:"mapped_value"`
	Unit        string     `json:"unit"`
	Scale       *float64   `json:"scale"`
	Source      string     `json:"source"`
	Status      string     `json:"status"`
	SampleAt    *time.Time `json:"sample_at"`
}

type Gyro struct {
	X        *float64   `json:"x"`
	Y        *float64   `json:"y"`
	Z        *float64   `json:"z"`
	YawRate  *float64   `json:"yaw_rate"`
	Unit     string     `json:"unit"`
	Source   string     `json:"source"`
	Status   string     `json:"status"`
	SampleAt *time.Time `json:"sample_at"`
}

type Lane struct {
	Valid       bool       `json:"valid"`
	OffsetRatio *float64   `json:"offset_ratio"`
	Direction   string     `json:"direction"`
	Confidence  *float64   `json:"confidence"`
	SampleAt    *time.Time `json:"sample_at"`
}

type Risk struct {
	Level        string   `json:"level"`
	ActiveEvents []string `json:"active_events"`
}

type Telemetry struct {
	SchemaVersion int       `json:"schema_version"`
	MessageID     string    `json:"message_id"`
	DeviceID      string    `json:"device_id"`
	Timestamp     time.Time `json:"timestamp"`
	Seq           *uint64   `json:"seq"`
	Speed         Sample    `json:"speed"`
	Distance      Distance  `json:"distance"`
	Temperature   Sample    `json:"temperature"`
	Gyro          Gyro      `json:"gyro"`
	Lane          Lane      `json:"lane"`
	Risk          Risk      `json:"risk"`
}

type Status struct {
	SchemaVersion   int               `json:"schema_version"`
	DeviceID        string            `json:"device_id"`
	Online          *bool             `json:"online"`
	Timestamp       *time.Time        `json:"timestamp"`
	SoftwareVersion *string           `json:"software_version"`
	ConfigVersion   *int              `json:"config_version"`
	Mode            *string           `json:"mode"`
	Sensors         map[string]string `json:"sensors"`
	MQTT            *struct {
		Connected *bool `json:"connected"`
	} `json:"mqtt"`
	Reason string `json:"reason,omitempty"`
}

func identity(topic, kind, deviceID string, version int) error {
	parts := strings.Split(topic, "/")
	if len(parts) != 3 || parts[0] != "car" || parts[2] != kind || parts[1] == "" ||
		len(parts[1]) > 64 || strings.ContainsAny(parts[1], "+#") || parts[1] != deviceID {
		return fmt.Errorf("topic and device_id do not match")
	}
	if version != 1 {
		return fmt.Errorf("unsupported schema_version: %d", version)
	}
	return nil
}

func oneOf(value string, choices ...string) bool {
	for _, choice := range choices {
		if value == choice {
			return true
		}
	}
	return false
}

func validSample(status, source, unit, expectedUnit string, at *time.Time) bool {
	return oneOf(status, "ok", "timeout", "unavailable", "error") &&
		oneOf(source, "mock", "hardware", "estimated", "replay") && unit == expectedUnit &&
		(at == nil || !at.IsZero())
}

func ParseTelemetry(topic string, payload []byte) (*Telemetry, error) {
	var t Telemetry
	if err := json.Unmarshal(payload, &t); err != nil {
		return nil, fmt.Errorf("telemetry JSON: %w", err)
	}
	if err := identity(topic, "telemetry", t.DeviceID, t.SchemaVersion); err != nil {
		return nil, err
	}
	if t.MessageID == "" || t.Timestamp.IsZero() || t.Seq == nil {
		return nil, fmt.Errorf("message_id, timestamp and seq are required")
	}
	if !validSample(t.Speed.Status, t.Speed.Source, t.Speed.Unit, "km/h", t.Speed.SampleAt) ||
		!validSample(t.Distance.Status, t.Distance.Source, t.Distance.Unit, "m", t.Distance.SampleAt) ||
		!validSample(t.Temperature.Status, t.Temperature.Source, t.Temperature.Unit, "C", t.Temperature.SampleAt) ||
		!validSample(t.Gyro.Status, t.Gyro.Source, t.Gyro.Unit, "deg/s", t.Gyro.SampleAt) ||
		!oneOf(t.Lane.Direction, "left", "center", "right", "unknown") ||
		!oneOf(t.Risk.Level, "normal", "warning", "danger", "unknown") ||
		(t.Lane.SampleAt != nil && t.Lane.SampleAt.IsZero()) ||
		(t.Lane.Confidence != nil && (*t.Lane.Confidence < 0 || *t.Lane.Confidence > 1)) ||
		(t.Distance.Scale != nil && *t.Distance.Scale <= 0) {
		return nil, fmt.Errorf("invalid telemetry sample metadata or enum")
	}
	if t.Risk.ActiveEvents == nil {
		t.Risk.ActiveEvents = []string{}
	}
	t.Timestamp = t.Timestamp.UTC()
	for _, at := range []*time.Time{t.Speed.SampleAt, t.Distance.SampleAt, t.Temperature.SampleAt, t.Gyro.SampleAt, t.Lane.SampleAt} {
		if at != nil {
			*at = at.UTC()
		}
	}
	return &t, nil
}

func ParseStatus(topic string, payload []byte) (*Status, error) {
	var s Status
	if err := json.Unmarshal(payload, &s); err != nil {
		return nil, fmt.Errorf("status JSON: %w", err)
	}
	if err := identity(topic, "status", s.DeviceID, s.SchemaVersion); err != nil {
		return nil, err
	}
	if s.Online == nil || (s.Timestamp != nil && s.Timestamp.IsZero()) || (s.ConfigVersion != nil && *s.ConfigVersion < 0) {
		return nil, fmt.Errorf("invalid status fields")
	}
	if *s.Online && (s.Timestamp == nil || s.SoftwareVersion == nil || s.ConfigVersion == nil ||
		s.Mode == nil || s.Sensors == nil || s.MQTT == nil || s.MQTT.Connected == nil) {
		return nil, fmt.Errorf("online status metadata is required")
	}
	if s.Mode != nil && !oneOf(*s.Mode, "mock", "hardware", "replay") {
		return nil, fmt.Errorf("invalid mode")
	}
	for _, status := range s.Sensors {
		if !oneOf(status, "ok", "timeout", "unavailable", "error") {
			return nil, fmt.Errorf("invalid sensor status")
		}
	}
	if s.Timestamp != nil {
		*s.Timestamp = s.Timestamp.UTC()
	}
	return &s, nil
}

type SampleInfo struct {
	SampleAt *time.Time `json:"sample_at"`
	Source   string     `json:"source"`
	Status   string     `json:"status"`
}

type State struct {
	DeviceID     string                `json:"device_id"`
	Timestamp    *time.Time            `json:"timestamp"`
	Online       bool                  `json:"online"`
	Speed        *float64              `json:"speed_kmh"`
	Distance     *float64              `json:"distance_m"`
	Temperature  *float64              `json:"temperature_c"`
	YawRate      *float64              `json:"yaw_rate_deg_s"`
	Lane         Lane                  `json:"lane"`
	RiskLevel    string                `json:"risk_level"`
	Samples      map[string]SampleInfo `json:"samples"`
	Freshness    map[string]string     `json:"freshness"`
	ActiveEvents []string              `json:"active_events"`
}

func freshness(at *time.Time, status string, usable bool, ttl time.Duration, now time.Time) string {
	if status != "" && status != "ok" {
		return "fault"
	}
	if !usable || at == nil {
		return "missing"
	}
	if now.Sub(*at) > ttl {
		return "stale"
	}
	return "fresh"
}

func usable(value *float64, status string, at *time.Time) *float64 {
	if status != "ok" || at == nil {
		return nil
	}
	return value
}

// View computes freshness at read time, rather than trusting a cached flag.
func View(deviceID string, online bool, t *Telemetry, now time.Time) State {
	s := State{DeviceID: deviceID, Online: online, RiskLevel: "unknown", ActiveEvents: []string{},
		Samples: map[string]SampleInfo{}, Freshness: map[string]string{}}
	for _, name := range []string{"speed", "distance", "temperature", "gyro", "lane"} {
		s.Samples[name] = SampleInfo{}
		s.Freshness[name] = "missing"
	}
	if t == nil {
		return s
	}
	s.Timestamp = &t.Timestamp
	s.Speed = usable(t.Speed.Value, t.Speed.Status, t.Speed.SampleAt)
	s.Distance = usable(t.Distance.MappedValue, t.Distance.Status, t.Distance.SampleAt)
	s.Temperature = usable(t.Temperature.Value, t.Temperature.Status, t.Temperature.SampleAt)
	s.YawRate = usable(t.Gyro.YawRate, t.Gyro.Status, t.Gyro.SampleAt)
	s.Lane = t.Lane
	if !s.Lane.Valid || s.Lane.SampleAt == nil || s.Lane.OffsetRatio == nil {
		s.Lane.Valid = false
		s.Lane.OffsetRatio = nil
	}
	s.RiskLevel, s.ActiveEvents = t.Risk.Level, t.Risk.ActiveEvents
	s.Samples["speed"] = SampleInfo{t.Speed.SampleAt, t.Speed.Source, t.Speed.Status}
	s.Samples["distance"] = SampleInfo{t.Distance.SampleAt, t.Distance.Source, t.Distance.Status}
	s.Samples["temperature"] = SampleInfo{t.Temperature.SampleAt, t.Temperature.Source, t.Temperature.Status}
	s.Samples["gyro"] = SampleInfo{t.Gyro.SampleAt, t.Gyro.Source, t.Gyro.Status}
	s.Samples["lane"] = SampleInfo{SampleAt: t.Lane.SampleAt}
	s.Freshness["speed"] = freshness(t.Speed.SampleAt, t.Speed.Status, s.Speed != nil, time.Second, now)
	s.Freshness["distance"] = freshness(t.Distance.SampleAt, t.Distance.Status, s.Distance != nil, time.Second, now)
	s.Freshness["temperature"] = freshness(t.Temperature.SampleAt, t.Temperature.Status, s.Temperature != nil, 5*time.Second, now)
	s.Freshness["gyro"] = freshness(t.Gyro.SampleAt, t.Gyro.Status, s.YawRate != nil, time.Second, now)
	s.Freshness["lane"] = freshness(t.Lane.SampleAt, "", s.Lane.Valid, time.Second, now)
	return s
}
