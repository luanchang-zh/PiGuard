package protocol

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"
	"unicode"
)

// Event 是一条已通过校验的告警消息。event_id 标识这一条消息，不标识可被后续动作改写的告警实例。
type Event struct {
	SchemaVersion     int             `json:"schema_version"`
	EventID           string          `json:"event_id"`
	DeviceID          string          `json:"device_id"`
	Type              string          `json:"type"`
	Action            string          `json:"action"`
	Level             string          `json:"level"`
	Timestamp         time.Time       `json:"timestamp"`
	Data              json.RawMessage `json:"data"`
	SnapshotID        *string         `json:"snapshot_id,omitempty"`
	SnapshotAvailable *bool           `json:"snapshot_available,omitempty"`
}

// EventItem 是 HTTP 告警列表中的一条记录。
type EventItem struct {
	EventID    string          `json:"event_id"`
	Type       string          `json:"type"`
	Action     string          `json:"action"`
	Level      string          `json:"level"`
	Timestamp  time.Time       `json:"timestamp"`
	Data       json.RawMessage `json:"data"`
	SnapshotID *string         `json:"snapshot_id"`
}

// EventPush 是 WebSocket event 的 data。类型字段沿用规范里的 event_type。
type EventPush struct {
	EventID    string          `json:"event_id"`
	DeviceID   string          `json:"device_id"`
	EventType  string          `json:"event_type"`
	Action     string          `json:"action"`
	Level      string          `json:"level"`
	Timestamp  time.Time       `json:"timestamp"`
	Data       json.RawMessage `json:"data"`
	SnapshotID *string         `json:"snapshot_id"`
}

type rawEvent struct {
	SchemaVersion     int             `json:"schema_version"`
	EventID           string          `json:"event_id"`
	DeviceID          string          `json:"device_id"`
	Type              string          `json:"type"`
	Action            string          `json:"action"`
	Level             string          `json:"level"`
	Timestamp         time.Time       `json:"timestamp"`
	Data              json.RawMessage `json:"data"`
	SnapshotID        json.RawMessage `json:"snapshot_id"`
	SnapshotAvailable json.RawMessage `json:"snapshot_available"`
}

func ValidEventType(v string) bool {
	return oneOf(v, "obstacle_warning", "lane_departure", "sharp_turn", "high_temperature", "sensor_failure")
}

func ValidEventAction(v string) bool {
	return oneOf(v, "started", "level_changed", "recovered")
}

func ValidEventLevel(v string) bool {
	return oneOf(v, "warning", "danger")
}

func validToken(id string) bool {
	return id != "" && len(id) <= 64 && !strings.ContainsFunc(id, unicode.IsSpace)
}

func ParseEvent(topic string, raw []byte) (*Event, error) {
	var in rawEvent
	if err := decodeObject(raw, &in); err != nil {
		return nil, err
	}
	if err := identity(topic, "events", in.DeviceID, in.SchemaVersion); err != nil {
		return nil, err
	}
	if !validToken(in.EventID) || !ValidEventType(in.Type) || !ValidEventAction(in.Action) || !ValidEventLevel(in.Level) || in.Timestamp.IsZero() || !jsonObject(in.Data) {
		return nil, fmt.Errorf("invalid event fields")
	}
	snapshot, err := optionalToken(in.SnapshotID)
	if err != nil {
		return nil, err
	}
	available, err := optionalBool(in.SnapshotAvailable)
	if err != nil {
		return nil, err
	}
	return &Event{
		SchemaVersion: in.SchemaVersion, EventID: in.EventID, DeviceID: in.DeviceID,
		Type: in.Type, Action: in.Action, Level: in.Level, Timestamp: in.Timestamp.UTC(),
		Data: append(json.RawMessage(nil), bytes.TrimSpace(in.Data)...), SnapshotID: snapshot, SnapshotAvailable: available,
	}, nil
}

func optionalToken(raw json.RawMessage) (*string, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, nil
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, fmt.Errorf("snapshot_id cannot be null")
	}
	var id string
	if err := json.Unmarshal(raw, &id); err != nil || !validToken(id) {
		return nil, fmt.Errorf("invalid snapshot_id")
	}
	return &id, nil
}

func optionalBool(raw json.RawMessage) (*bool, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, nil
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, fmt.Errorf("snapshot_available cannot be null")
	}
	var value bool
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("snapshot_available must be boolean")
	}
	return &value, nil
}

// SameRecord reports whether this message is a replay of an already stored row.
// snapshot_available is intentionally ignored.
func (e Event) SameRecord(deviceID, eventType, action, level string, at time.Time, data, snapshotID string) bool {
	current := ""
	if e.SnapshotID != nil {
		current = *e.SnapshotID
	}
	return e.DeviceID == deviceID && e.Type == eventType && e.Action == action && e.Level == level &&
		e.Timestamp.UTC().Equal(at.UTC()) && current == snapshotID && jsonEqual(e.Data, []byte(data))
}

func jsonEqual(left, right []byte) bool {
	var a, b any
	if json.Unmarshal(left, &a) != nil || json.Unmarshal(right, &b) != nil {
		return false
	}
	return reflect.DeepEqual(a, b)
}
