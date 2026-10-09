package simulator

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"piguard/go-backend/internal/protocol"
)

func TestPublishDemoEventRepeatsSameQoS1Message(t *testing.T) {
	at := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	var calls []struct {
		kind     string
		qos      byte
		retained bool
		raw      []byte
	}
	err := PublishDemoEvent(func(kind string, data any, qos byte, retained bool) error {
		raw, err := json.Marshal(data)
		if err != nil {
			return err
		}
		calls = append(calls, struct {
			kind     string
			qos      byte
			retained bool
			raw      []byte
		}{kind, qos, retained, raw})
		return nil
	}, "car-001", "evt-sim-1", at)
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 || calls[0].kind != "events" || calls[0].qos != 1 || calls[0].retained || !bytes.Equal(calls[0].raw, calls[1].raw) {
		t.Fatal(calls)
	}
	first, err := protocol.ParseEvent("car/car-001/events", calls[0].raw)
	if err != nil {
		t.Fatal(err)
	}
	second, err := protocol.ParseEvent("car/car-001/events", calls[1].raw)
	if err != nil || first.EventID != "evt-sim-1" || !second.SameRecord(first.DeviceID, first.Type, first.Action, first.Level, first.Timestamp, string(first.Data), "") {
		t.Fatal(second, err)
	}
}
