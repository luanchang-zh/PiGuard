package simulator

import (
	"fmt"
	"sync"
	"time"

	"piguard/go-backend/internal/model"
	"piguard/go-backend/internal/protocol"
)

type configResult struct {
	message protocol.ConfigMessage
	ack     *protocol.ConfigAck
}
type ConfigExecutor struct {
	mu           sync.Mutex
	mode         string
	results      map[string]map[int]configResult
	versions     map[string]int
	rules        map[string]model.RuleSet
	applications int
}

func NewConfigExecutor(mode string) (*ConfigExecutor, error) {
	if mode != "success" && mode != "failed" && mode != "none" {
		return nil, fmt.Errorf("config-ack-mode must be success, failed or none")
	}
	return &ConfigExecutor{mode: mode, results: map[string]map[int]configResult{}, versions: map[string]int{}, rules: map[string]model.RuleSet{}}, nil
}
func (e *ConfigExecutor) Apply(topic string, raw []byte, at time.Time) (*protocol.ConfigAck, bool, error) {
	c, err := protocol.ParseConfigMessage(topic, raw)
	if err != nil {
		return nil, false, err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.results[c.DeviceID] == nil {
		e.results[c.DeviceID] = map[int]configResult{}
	}
	if old, ok := e.results[c.DeviceID][c.ConfigVersion]; ok {
		if old.message.Rules != c.Rules || !old.message.IssuedAt.Equal(c.IssuedAt) {
			return nil, false, fmt.Errorf("conflicting config version")
		}
		return old.ack, false, nil
	}
	if c.ConfigVersion < e.versions[c.DeviceID] {
		return nil, false, fmt.Errorf("stale config version")
	}
	ack := &protocol.ConfigAck{SchemaVersion: 1, DeviceID: c.DeviceID, ConfigVersion: c.ConfigVersion}
	applied := false
	if e.mode == "failed" {
		ack.Status = "failed"
		ack.Error = &protocol.ConfigError{Code: "INVALID_CONFIG", Message: "simulated device rejection"}
	} else {
		e.versions[c.DeviceID] = c.ConfigVersion
		e.rules[c.DeviceID] = c.Rules
		e.applications++
		applied = true
		if e.mode == "none" {
			ack = nil
		} else {
			now := at.UTC()
			ack.Status = "success"
			ack.AppliedAt = &now
		}
	}
	e.results[c.DeviceID][c.ConfigVersion] = configResult{message: *c, ack: ack}
	return ack, applied, nil
}
func (e *ConfigExecutor) Version(id string) int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.versions[id]
}
func (e *ConfigExecutor) Applications() int { e.mu.Lock(); defer e.mu.Unlock(); return e.applications }
