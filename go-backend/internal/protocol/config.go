package protocol

import (
	"bytes"
	"encoding/json"
	"math"
	"strings"
	"time"

	"piguard/go-backend/internal/apperr"
	"piguard/go-backend/internal/model"
)

type ConfigRequest struct {
	ExpectedVersion int             `json:"expected_version"`
	Rules           json.RawMessage `json:"rules"`
}

type ConfigMessage struct {
	SchemaVersion int           `json:"schema_version"`
	DeviceID      string        `json:"device_id"`
	ConfigVersion int           `json:"config_version"`
	IssuedAt      time.Time     `json:"issued_at"`
	Rules         model.RuleSet `json:"rules"`
}

type ConfigError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
type ConfigAck struct {
	SchemaVersion int          `json:"schema_version"`
	DeviceID      string       `json:"device_id"`
	ConfigVersion int          `json:"config_version"`
	Status        string       `json:"status"`
	AppliedAt     *time.Time   `json:"applied_at,omitempty"`
	Error         *ConfigError `json:"error,omitempty"`
}
type ConfigView struct {
	DesiredVersion  int           `json:"desired_version"`
	ReportedVersion int           `json:"reported_version"`
	Rules           model.RuleSet `json:"rules"`
	Status          string        `json:"status"`
	AckAt           *time.Time    `json:"ack_at"`
	AppliedAt       *time.Time    `json:"applied_at"`
	Error           *ConfigError  `json:"error"`
}

func configInvalid(field string) error { return &apperr.InvalidParams{Field: field} }
func configFields(raw []byte, path string, allowed ...string) (map[string]json.RawMessage, error) {
	if !jsonObject(raw) {
		return nil, configInvalid(path)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, configInvalid(path)
	}
	for name, value := range fields {
		ok := false
		for _, a := range allowed {
			if name == a {
				ok = true
				break
			}
		}
		field := name
		if path != "body" {
			field = path + "." + name
		}
		if !ok || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return nil, configInvalid(field)
		}
	}
	return fields, nil
}

var configRuleFields = map[string][]string{
	"obstacle":       {"warning_distance_m", "danger_distance_m"},
	"lane_departure": {"offset_threshold", "duration_ms"},
	"sharp_turn":     {"yaw_threshold_deg_s", "min_speed_kmh", "duration_ms"},
	"temperature":    {"trigger_c", "recover_c", "duration_ms"},
}

func rulesPatch(raw []byte, full bool) (map[string]map[string]json.RawMessage, error) {
	groups, err := configFields(raw, "rules", "obstacle", "lane_departure", "sharp_turn", "temperature")
	if err != nil {
		return nil, err
	}
	if len(groups) == 0 {
		return nil, configInvalid("rules")
	}
	patch := map[string]map[string]json.RawMessage{}
	for group, names := range configRuleFields {
		value, ok := groups[group]
		if !ok {
			if full {
				return nil, configInvalid("rules." + group)
			}
			continue
		}
		fields, err := configFields(value, "rules."+group, names...)
		if err != nil {
			return nil, err
		}
		if len(fields) == 0 {
			return nil, configInvalid("rules." + group)
		}
		for _, name := range names {
			v, exists := fields[name]
			if !exists {
				if full {
					return nil, configInvalid("rules." + group + "." + name)
				}
				continue
			}
			path := "rules." + group + "." + name
			if name == "duration_ms" {
				var n int
				if json.Unmarshal(v, &n) != nil || n <= 0 {
					return nil, configInvalid(path)
				}
			} else {
				var n float64
				if json.Unmarshal(v, &n) != nil || math.IsNaN(n) || math.IsInf(n, 0) {
					return nil, configInvalid(path)
				}
			}
		}
		patch[group] = fields
	}
	return patch, nil
}

func ParseConfigRequest(raw []byte) (*ConfigRequest, error) {
	if _, err := configFields(raw, "body", "expected_version", "rules"); err != nil {
		return nil, err
	}
	var r ConfigRequest
	if err := decodeObject(raw, &r); err != nil {
		return nil, configInvalid("body")
	}
	if err := r.Validate(); err != nil {
		return nil, err
	}
	return &r, nil
}
func (r ConfigRequest) Validate() error {
	if r.ExpectedVersion <= 0 || r.ExpectedVersion == int(^uint(0)>>1) {
		return configInvalid("expected_version")
	}
	_, err := rulesPatch(r.Rules, false)
	return err
}
func (r ConfigRequest) Merge(base model.RuleSet) (model.RuleSet, error) {
	if err := r.Validate(); err != nil {
		return base, err
	}
	patch, err := rulesPatch(r.Rules, false)
	if err != nil {
		return base, err
	}
	raw, _ := json.Marshal(base)
	var groups map[string]map[string]json.RawMessage
	if err := json.Unmarshal(raw, &groups); err != nil {
		return base, err
	}
	for name, fields := range patch {
		for field, value := range fields {
			groups[name][field] = value
		}
	}
	raw, err = json.Marshal(groups)
	if err != nil {
		return base, err
	}
	var merged model.RuleSet
	if err := json.Unmarshal(raw, &merged); err != nil {
		return base, configInvalid("rules")
	}
	return merged, ValidateRules(merged)
}

func ValidateRules(r model.RuleSet) error {
	values := []float64{r.Obstacle.WarningDistanceM, r.Obstacle.DangerDistanceM, r.LaneDeparture.OffsetThreshold, r.SharpTurn.YawThresholdDegS, r.SharpTurn.MinSpeedKmh, r.Temperature.TriggerC, r.Temperature.RecoverC}
	for _, v := range values {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return configInvalid("rules")
		}
	}
	if r.Obstacle.WarningDistanceM <= 0 {
		return configInvalid("rules.obstacle.warning_distance_m")
	}
	if r.Obstacle.DangerDistanceM <= 0 || r.Obstacle.DangerDistanceM >= r.Obstacle.WarningDistanceM {
		return configInvalid("rules.obstacle.danger_distance_m")
	}
	if r.LaneDeparture.OffsetThreshold <= 0 || r.LaneDeparture.OffsetThreshold > 1 {
		return configInvalid("rules.lane_departure.offset_threshold")
	}
	if r.LaneDeparture.DurationMS <= 0 {
		return configInvalid("rules.lane_departure.duration_ms")
	}
	if r.SharpTurn.YawThresholdDegS <= 0 {
		return configInvalid("rules.sharp_turn.yaw_threshold_deg_s")
	}
	if r.SharpTurn.MinSpeedKmh < 0 {
		return configInvalid("rules.sharp_turn.min_speed_kmh")
	}
	if r.SharpTurn.DurationMS <= 0 {
		return configInvalid("rules.sharp_turn.duration_ms")
	}
	if r.Temperature.RecoverC >= r.Temperature.TriggerC {
		return configInvalid("rules.temperature.recover_c")
	}
	if r.Temperature.DurationMS <= 0 {
		return configInvalid("rules.temperature.duration_ms")
	}
	return nil
}

func ParseConfigMessage(topic string, raw []byte) (*ConfigMessage, error) {
	fields, err := configFields(raw, "body", "schema_version", "device_id", "config_version", "issued_at", "rules")
	if err != nil {
		return nil, err
	}
	if _, err = rulesPatch(fields["rules"], true); err != nil {
		return nil, err
	}
	var c ConfigMessage
	if err = decodeObject(raw, &c); err != nil {
		return nil, err
	}
	if err = identity(topic, "config", c.DeviceID, c.SchemaVersion); err != nil {
		return nil, err
	}
	if c.ConfigVersion <= 0 {
		return nil, configInvalid("config_version")
	}
	if c.IssuedAt.IsZero() {
		return nil, configInvalid("issued_at")
	}
	if err = ValidateRules(c.Rules); err != nil {
		return nil, err
	}
	c.IssuedAt = c.IssuedAt.UTC()
	return &c, nil
}

func ParseConfigAck(topic string, raw []byte) (*ConfigAck, error) {
	fields, err := configFields(raw, "body", "schema_version", "device_id", "config_version", "status", "applied_at", "error")
	if err != nil {
		return nil, err
	}
	var a ConfigAck
	if err = decodeObject(raw, &a); err != nil {
		return nil, err
	}
	if err = identity(topic, "config-acks", a.DeviceID, a.SchemaVersion); err != nil {
		return nil, err
	}
	if a.ConfigVersion <= 0 {
		return nil, configInvalid("config_version")
	}
	switch a.Status {
	case "success":
		if a.AppliedAt == nil || a.AppliedAt.IsZero() || a.Error != nil {
			return nil, configInvalid("applied_at")
		}
		at := a.AppliedAt.UTC()
		a.AppliedAt = &at
	case "failed":
		if a.AppliedAt != nil || a.Error == nil {
			return nil, configInvalid("error")
		}
		if _, err = configFields(fields["error"], "error", "code", "message"); err != nil {
			return nil, err
		}
		if strings.TrimSpace(a.Error.Code) == "" || strings.TrimSpace(a.Error.Message) == "" {
			return nil, configInvalid("error")
		}
	default:
		return nil, configInvalid("status")
	}
	return &a, nil
}
