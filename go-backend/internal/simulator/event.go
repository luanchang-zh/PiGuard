package simulator

import (
	"encoding/json"
	"time"

	"piguard/go-backend/internal/protocol"
)

// DemoEvent 是模拟器启动时发布的一条固定告警。同一 event_id 的再次发布必须使用同一内容。
func DemoEvent(deviceID, eventID string, at time.Time) protocol.Event {
	return protocol.Event{
		SchemaVersion: 1, EventID: eventID, DeviceID: deviceID, Type: "obstacle_warning",
		Action: "started", Level: "warning", Timestamp: at.UTC(),
		Data: json.RawMessage(`{"distance_m":6.8,"speed_kmh":32}`),
	}
}

// PublishDemoEvent 以 QoS 1、非 retained 发布告警，并立即重发同一条消息。
func PublishDemoEvent(publish func(kind string, data any, qos byte, retained bool) error, deviceID, eventID string, at time.Time) error {
	event := DemoEvent(deviceID, eventID, at)
	if err := publish("events", event, 1, false); err != nil {
		return err
	}
	return publish("events", event, 1, false)
}
