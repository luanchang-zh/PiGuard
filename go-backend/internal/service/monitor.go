package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"sync"
	"time"

	"piguard/go-backend/internal/apperr"
	"piguard/go-backend/internal/model"
	"piguard/go-backend/internal/protocol"
	"piguard/go-backend/internal/realtime"
	"piguard/go-backend/internal/repo"
)

type currentState struct {
	telemetry    *protocol.Telemetry
	online       bool
	lastWrite    time.Time
	freshness    map[string]string
	historyKnown bool
	lastSnapshot *model.Telemetry
}

// Monitor serializes state transitions; Hub never waits for a client to read.
type Monitor struct {
	mu      sync.Mutex
	devices repo.DeviceRepository
	history repo.TelemetryRepository
	hub     *realtime.Hub
	states  map[string]*currentState
	now     func() time.Time
}

func NewMonitor(devices repo.DeviceRepository, history repo.TelemetryRepository, hub *realtime.Hub) *Monitor {
	return &Monitor{devices: devices, history: history, hub: hub, states: map[string]*currentState{}, now: time.Now}
}

func (m *Monitor) requireDevice(ctx context.Context, id string) (*model.Device, error) {
	d, err := m.devices.Find(ctx, id)
	if errors.Is(err, repo.ErrNotFound) {
		return nil, apperr.ErrDeviceNotFound
	}
	return d, err
}

func (m *Monitor) state(id string) *currentState {
	s, ok := m.states[id]
	if !ok {
		s = &currentState{}
		m.states[id] = s
	}
	return s
}

func (m *Monitor) State(ctx context.Context, id string) (protocol.State, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, err := m.requireDevice(ctx, id); err != nil {
		return protocol.State{}, err
	}
	s := m.state(id)
	return protocol.View(id, s.online, s.telemetry, m.now()), nil
}

func (m *Monitor) Ingest(ctx context.Context, topic string, payload []byte) error {
	switch {
	case len(topic) >= 10 && topic[len(topic)-10:] == "/telemetry":
		t, err := protocol.ParseTelemetry(topic, payload)
		if err != nil {
			return err
		}
		return m.telemetry(ctx, t)
	case len(topic) >= 7 && topic[len(topic)-7:] == "/status":
		s, err := protocol.ParseStatus(topic, payload)
		if err != nil {
			return err
		}
		return m.status(ctx, s)
	default:
		return fmt.Errorf("unsupported uplink topic")
	}
}

func (m *Monitor) telemetry(ctx context.Context, t *protocol.Telemetry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, err := m.requireDevice(ctx, t.DeviceID); err != nil {
		return err
	}
	s := m.state(t.DeviceID)
	if previous := s.telemetry; previous != nil && (t.Timestamp.Before(previous.Timestamp) ||
		(t.Timestamp.Equal(previous.Timestamp) && *t.Seq <= *previous.Seq)) {
		return nil
	}
	now := m.now().UTC()
	if err := m.devices.Update(ctx, t.DeviceID, map[string]any{"last_seen_at": now}); err != nil {
		return err
	}
	s.telemetry = t
	view := protocol.View(t.DeviceID, s.online, t, now)
	s.freshness = maps.Clone(view.Freshness)
	m.hub.Publish("telemetry", view, now)
	// Restore only the history watermark; never replay it as realtime state.
	if !s.historyKnown {
		row, err := m.history.Latest(ctx, t.DeviceID)
		if err != nil {
			return fmt.Errorf("read history watermark: %w", err)
		}
		s.historyKnown = true
		s.lastSnapshot = row
		if row != nil {
			s.lastWrite = row.ReceivedAt
		}
	}
	if previous := s.lastSnapshot; previous != nil && (t.Timestamp.Before(previous.Timestamp) ||
		(t.Timestamp.Equal(previous.Timestamp) && *t.Seq <= previous.Seq)) {
		return nil
	}
	if !s.lastWrite.IsZero() && now.Sub(s.lastWrite) < time.Second {
		return nil
	}
	raw, err := json.Marshal(t)
	if err != nil {
		return err
	}
	row := &model.Telemetry{DeviceID: t.DeviceID, Timestamp: t.Timestamp, Seq: *t.Seq,
		Speed: view.Speed, Distance: view.Distance, Temperature: view.Temperature, YawRate: view.YawRate,
		LaneOffset: view.Lane.OffsetRatio, RiskLevel: view.RiskLevel, PayloadJSON: string(raw), ReceivedAt: now}
	if t.Gyro.Status == "ok" && t.Gyro.SampleAt != nil {
		row.GyroX, row.GyroY, row.GyroZ = t.Gyro.X, t.Gyro.Y, t.Gyro.Z
	}
	if err := m.history.Insert(ctx, row); err != nil {
		return fmt.Errorf("save telemetry history: %w", err)
	}
	s.lastWrite = now
	s.lastSnapshot = row
	return nil
}

func (m *Monitor) status(ctx context.Context, report *protocol.Status) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, err := m.requireDevice(ctx, report.DeviceID); err != nil {
		return err
	}
	now := m.now().UTC()
	fields := map[string]any{"online": *report.Online, "last_seen_at": now}
	if report.Mode != nil {
		fields["mode"] = *report.Mode
	}
	if report.SoftwareVersion != nil {
		fields["software_version"] = *report.SoftwareVersion
	}
	if report.ConfigVersion != nil {
		fields["reported_config_version"] = *report.ConfigVersion
	}
	if report.Sensors != nil {
		raw, _ := json.Marshal(report.Sensors)
		fields["sensors_json"] = string(raw)
	}
	if report.Timestamp != nil {
		raw, _ := json.Marshal(report)
		fields["status_json"] = string(raw)
	}
	if err := m.devices.Update(ctx, report.DeviceID, fields); err != nil {
		return err
	}
	s := m.state(report.DeviceID)
	changed := s.online != *report.Online
	s.online = *report.Online
	if changed {
		m.hub.Publish("device_status", map[string]any{"device_id": report.DeviceID, "online": s.online}, now)
		if s.telemetry != nil {
			m.hub.Publish("telemetry", protocol.View(report.DeviceID, s.online, s.telemetry, now), now)
		}
	}
	return nil
}

// Disconnected invalidates online facts without clearing configuration/history.
func (m *Monitor) Disconnected(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	err := m.devices.MarkAllOffline(ctx)
	for id, s := range m.states {
		if !s.online {
			continue
		}
		s.online = false
		m.hub.Publish("device_status", map[string]any{"device_id": id, "online": false}, m.now())
		if s.telemetry != nil {
			m.hub.Publish("telemetry", protocol.View(id, false, s.telemetry, m.now()), m.now())
		}
	}
	return err
}

func (m *Monitor) Online(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.states[id]
	return s != nil && s.online
}

func (m *Monitor) Expire() {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.now()
	for id, s := range m.states {
		if s.telemetry == nil {
			continue
		}
		view := protocol.View(id, s.online, s.telemetry, now)
		if !maps.Equal(s.freshness, view.Freshness) {
			s.freshness = maps.Clone(view.Freshness)
			m.hub.Publish("telemetry", view, now)
		}
	}
}

func (m *Monitor) Run(ctx context.Context) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.Expire()
		}
	}
}

// LogIngest isolates malformed MQTT input from the process lifecycle.
func (m *Monitor) LogIngest(ctx context.Context, topic string, payload []byte) {
	if err := m.Ingest(ctx, topic, payload); err != nil {
		slog.Warn("MQTT 上行处理失败", "topic", topic, "err", err)
	}
}
