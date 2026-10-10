package protocol

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

type CommandRequest struct {
	Type   string          `json:"type"`
	Params json.RawMessage `json:"params"`
}
type SnapshotParams struct {
	Quality int `json:"quality"`
}
type SnapshotResult struct {
	SnapshotID string `json:"snapshot_id"`
}

func ParseSnapshotParams(raw []byte) (SnapshotParams, error) {
	var fields struct {
		Quality json.RawMessage `json:"quality"`
	}
	if err := decodeObject(raw, &fields); err != nil {
		return SnapshotParams{}, err
	}
	p := SnapshotParams{Quality: 80}
	if len(fields.Quality) != 0 {
		if bytes.Equal(bytes.TrimSpace(fields.Quality), []byte("null")) {
			return SnapshotParams{}, fmt.Errorf("quality must be an integer from 1 to 100")
		}
		if err := json.Unmarshal(fields.Quality, &p.Quality); err != nil {
			return SnapshotParams{}, err
		}
	}
	if p.Quality < 1 || p.Quality > 100 {
		return SnapshotParams{}, fmt.Errorf("quality must be 1..100")
	}
	return p, nil
}

func ParseSnapshotResult(raw []byte) (SnapshotResult, error) {
	var r SnapshotResult
	if err := decodeObject(raw, &r); err != nil {
		return r, err
	}
	if !validCommandID(r.SnapshotID) || strings.TrimSpace(r.SnapshotID) != r.SnapshotID {
		return r, fmt.Errorf("snapshot_id is required")
	}
	return r, nil
}

type Command struct {
	SchemaVersion int             `json:"schema_version"`
	CommandID     string          `json:"command_id"`
	DeviceID      string          `json:"device_id"`
	Type          string          `json:"type"`
	IssuedAt      time.Time       `json:"issued_at"`
	ExpiresAt     time.Time       `json:"expires_at"`
	Params        json.RawMessage `json:"params"`
}
type CommandError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
type CommandAck struct {
	SchemaVersion int             `json:"schema_version"`
	CommandID     string          `json:"command_id"`
	DeviceID      string          `json:"device_id"`
	Status        string          `json:"status"`
	ExecutedAt    time.Time       `json:"executed_at"`
	Result        json.RawMessage `json:"result,omitempty"`
	Error         *CommandError   `json:"error,omitempty"`
}
type CommandView struct {
	CommandID  string          `json:"command_id"`
	DeviceID   string          `json:"device_id"`
	Type       string          `json:"type"`
	Params     json.RawMessage `json:"params"`
	Status     string          `json:"status"`
	IssuedAt   time.Time       `json:"issued_at"`
	ExpiresAt  time.Time       `json:"expires_at"`
	AckAt      *time.Time      `json:"ack_at"`
	ExecutedAt *time.Time      `json:"executed_at"`
	Result     json.RawMessage `json:"result"`
	Error      json.RawMessage `json:"error"`
}

func jsonObject(raw []byte) bool {
	raw = bytes.TrimSpace(raw)
	return len(raw) > 1 && raw[0] == '{' && json.Valid(raw)
}
func decodeObject(raw []byte, dst any) error {
	if !jsonObject(raw) {
		return fmt.Errorf("expected JSON object")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(dst); err != nil {
		return err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("expected one JSON object")
	}
	return nil
}
func ParseCommandRequest(raw []byte) (*CommandRequest, error) {
	var r CommandRequest
	if err := decodeObject(raw, &r); err != nil {
		return nil, err
	}
	if err := r.Validate(); err != nil {
		return nil, err
	}
	return &r, nil
}
func (r CommandRequest) Validate() error {
	switch r.Type {
	case "buzzer.test":
		var p struct {
			Duration *int `json:"duration_ms"`
		}
		if err := decodeObject(r.Params, &p); err != nil {
			return err
		}
		if p.Duration == nil || *p.Duration < 100 || *p.Duration > 5000 {
			return fmt.Errorf("duration_ms must be 100..5000")
		}
	case "indicator.test":
		var p struct {
			Duration *int   `json:"duration_ms"`
			Color    string `json:"color"`
		}
		if err := decodeObject(r.Params, &p); err != nil {
			return err
		}
		if p.Duration == nil || *p.Duration < 100 || *p.Duration > 5000 || !oneOf(p.Color, "green", "yellow", "red") {
			return fmt.Errorf("invalid indicator params")
		}
	case "scenario.start":
		var p struct {
			Scenario string   `json:"scenario"`
			Speed    *float64 `json:"speed"`
		}
		if err := decodeObject(r.Params, &p); err != nil {
			return err
		}
		if !oneOf(p.Scenario, "normal_drive", "obstacle_approach", "lane_departure", "sharp_turn", "high_temperature", "sensor_failure") || p.Speed == nil || (*p.Speed != 0.5 && *p.Speed != 1 && *p.Speed != 2) {
			return fmt.Errorf("invalid scenario params")
		}
	case "scenario.stop":
		if err := decodeObject(r.Params, &struct{}{}); err != nil {
			return err
		}
	case "camera.snapshot":
		_, err := ParseSnapshotParams(r.Params)
		return err
	default:
		return fmt.Errorf("unsupported command type")
	}
	return nil
}

func (r CommandRequest) NormalizedParams() (json.RawMessage, error) {
	if err := r.Validate(); err != nil {
		return nil, err
	}
	if r.Type == "camera.snapshot" {
		p, err := ParseSnapshotParams(r.Params)
		if err != nil {
			return nil, err
		}
		return json.Marshal(p)
	}
	return append(json.RawMessage(nil), r.Params...), nil
}
func validCommandID(id string) bool { return strings.TrimSpace(id) != "" && len(id) <= 64 }
func ParseCommand(topic string, raw []byte) (*Command, error) {
	var c Command
	if err := decodeObject(raw, &c); err != nil {
		return nil, err
	}
	if err := identity(topic, "commands", c.DeviceID, c.SchemaVersion); err != nil {
		return nil, err
	}
	if !validCommandID(c.CommandID) || c.IssuedAt.IsZero() || !c.ExpiresAt.After(c.IssuedAt) {
		return nil, fmt.Errorf("invalid command identity or timestamps")
	}
	params, err := (CommandRequest{Type: c.Type, Params: c.Params}).NormalizedParams()
	if err != nil {
		return nil, err
	}
	c.Params = params
	c.IssuedAt, c.ExpiresAt = c.IssuedAt.UTC(), c.ExpiresAt.UTC()
	return &c, nil
}
func ParseCommandAck(topic string, raw []byte) (*CommandAck, error) {
	var a CommandAck
	if err := decodeObject(raw, &a); err != nil {
		return nil, err
	}
	if err := identity(topic, "command-acks", a.DeviceID, a.SchemaVersion); err != nil {
		return nil, err
	}
	_, offset := a.ExecutedAt.Zone()
	if !validCommandID(a.CommandID) || a.ExecutedAt.IsZero() || offset != 0 {
		return nil, fmt.Errorf("command_id and UTC executed_at are required")
	}
	switch a.Status {
	case "success":
		if !jsonObject(a.Result) || a.Error != nil {
			return nil, fmt.Errorf("success requires result only")
		}
	case "failed":
		if a.Error == nil || strings.TrimSpace(a.Error.Code) == "" || strings.TrimSpace(a.Error.Message) == "" || len(a.Result) != 0 {
			return nil, fmt.Errorf("failed requires error only")
		}
	default:
		return nil, fmt.Errorf("unsupported Ack status")
	}
	a.ExecutedAt = a.ExecutedAt.UTC()
	return &a, nil
}
