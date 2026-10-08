package testutil

import (
	"encoding/json"
	"piguard/go-backend/internal/protocol"
	"time"
)

func Ptr[T any](v T) *T { return &v }

func Telemetry(id string, at time.Time, seq uint64) *protocol.Telemetry {
	return &protocol.Telemetry{SchemaVersion: 1, MessageID: "test-message", DeviceID: id, Timestamp: at, Seq: Ptr(seq),
		Speed:       protocol.Sample{Value: Ptr(32.4), Unit: "km/h", Source: "mock", Status: "ok", SampleAt: &at},
		Distance:    protocol.Distance{RawValue: Ptr(.18), MappedValue: Ptr(18.0), Unit: "m", Scale: Ptr(100.0), Source: "mock", Status: "ok", SampleAt: &at},
		Temperature: protocol.Sample{Value: Ptr(27.5), Unit: "C", Source: "mock", Status: "ok", SampleAt: &at},
		Gyro:        protocol.Gyro{X: Ptr(.6), Y: Ptr(1.2), Z: Ptr(12.8), YawRate: Ptr(12.8), Unit: "deg/s", Source: "mock", Status: "ok", SampleAt: &at},
		Lane:        protocol.Lane{Valid: true, OffsetRatio: Ptr(.18), Direction: "right", Confidence: Ptr(.91), SampleAt: &at},
		Risk:        protocol.Risk{Level: "normal", ActiveEvents: []string{}},
	}
}

func Status(id string, at time.Time, online bool) *protocol.Status {
	s := &protocol.Status{SchemaVersion: 1, DeviceID: id, Online: &online, Reason: "connection_lost"}
	if online {
		s.Timestamp = &at
		s.SoftwareVersion = Ptr("0.2.0")
		s.ConfigVersion = Ptr(5)
		s.Mode = Ptr("mock")
		s.Sensors = map[string]string{"distance": "ok", "camera": "ok", "gyro": "ok", "temperature": "ok"}
		// The anonymous field's JSON representation is easier to share through decoding.
		_ = json.Unmarshal([]byte(`{"connected":true}`), &s.MQTT)
	}
	return s
}

func JSON(v any) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return data
}
