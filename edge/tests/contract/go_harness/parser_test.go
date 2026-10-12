package protocol

import (
	"encoding/json"
	"os"
	"testing"
)

type fixtureFile struct {
	AcceptTelemetry []fixture `json:"accept_telemetry"`
	RejectTelemetry []fixture `json:"reject_telemetry"`
	AcceptStatus    []fixture `json:"accept_status"`
	RejectStatus    []fixture `json:"reject_status"`
	AcceptEvent     []fixture `json:"accept_event"`
	RejectEvent     []fixture `json:"reject_event"`
	AcceptCommand   []fixture `json:"accept_command"`
	RejectCommand   []fixture `json:"reject_command"`
	AcceptAck       []fixture `json:"accept_ack"`
	RejectAck       []fixture `json:"reject_ack"`
}

type fixture struct {
	Topic string          `json:"topic"`
	Body  json.RawMessage `json:"body"`
}

func TestFixtures(t *testing.T) {
	raw, err := os.ReadFile(os.Getenv("PAYLOADS"))
	if err != nil {
		t.Fatal(err)
	}
	var file fixtureFile
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatal(err)
	}
	for i, item := range file.AcceptTelemetry {
		if _, err := ParseTelemetry(item.Topic, item.Body); err != nil {
			t.Fatalf("accept telemetry %d: %v\n%s", i, err, item.Body)
		}
	}
	for i, item := range file.RejectTelemetry {
		if _, err := ParseTelemetry(item.Topic, item.Body); err == nil {
			t.Fatalf("reject telemetry %d was accepted\n%s", i, item.Body)
		}
	}
	for i, item := range file.AcceptStatus {
		if _, err := ParseStatus(item.Topic, item.Body); err != nil {
			t.Fatalf("accept status %d: %v\n%s", i, err, item.Body)
		}
	}
	for i, item := range file.RejectStatus {
		if _, err := ParseStatus(item.Topic, item.Body); err == nil {
			t.Fatalf("reject status %d was accepted\n%s", i, item.Body)
		}
	}
	for i, item := range file.AcceptEvent {
		if _, err := ParseEvent(item.Topic, item.Body); err != nil {
			t.Fatalf("accept event %d: %v\n%s", i, err, item.Body)
		}
	}
	for i, item := range file.RejectEvent {
		if _, err := ParseEvent(item.Topic, item.Body); err == nil {
			t.Fatalf("reject event %d was accepted\n%s", i, item.Body)
		}
	}
	for i, item := range file.AcceptCommand {
		if _, err := ParseCommand(item.Topic, item.Body); err != nil {
			t.Fatalf("accept command %d: %v\n%s", i, err, item.Body)
		}
	}
	for i, item := range file.RejectCommand {
		if _, err := ParseCommand(item.Topic, item.Body); err == nil {
			t.Fatalf("reject command %d was accepted\n%s", i, item.Body)
		}
	}
	for i, item := range file.AcceptAck {
		if _, err := ParseCommandAck(item.Topic, item.Body); err != nil {
			t.Fatalf("accept ack %d: %v\n%s", i, err, item.Body)
		}
	}
	for i, item := range file.RejectAck {
		if _, err := ParseCommandAck(item.Topic, item.Body); err == nil {
			t.Fatalf("reject ack %d was accepted\n%s", i, item.Body)
		}
	}
}
