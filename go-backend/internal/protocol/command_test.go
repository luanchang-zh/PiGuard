package protocol

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestCommandRequests(t *testing.T) {
	for _, body := range []string{
		`{"type":"buzzer.test","params":{"duration_ms":100}}`,
		`{"type":"buzzer.test","params":{"duration_ms":5000}}`,
		`{"type":"indicator.test","params":{"color":"red","duration_ms":1000}}`,
		`{"type":"scenario.start","params":{"scenario":"sensor_failure","speed":0.5}}`,
		`{"type":"scenario.stop","params":{}}`,
	} {
		if _, err := ParseCommandRequest([]byte(body)); err != nil {
			t.Fatal(body, err)
		}
	}
	for _, body := range []string{
		`null`, `[]`, `{}`, `{"type":"buzzer.test","params":null}`,
		`{"type":"buzzer.test","params":{"duration_ms":99}}`,
		`{"type":"buzzer.test","params":{"duration_ms":5001}}`,
		`{"type":"buzzer.test","params":{"duration_ms":100.5}}`,
		`{"type":"buzzer.test","params":{"duration_ms":"100"}}`,
		`{"type":"buzzer.test","params":{"duration_ms":100,"extra":1}}`,
		`{"type":"indicator.test","params":{"color":"blue","duration_ms":100}}`,
		`{"type":"indicator.test","params":{"color":"green"}}`,
		`{"type":"scenario.start","params":{"scenario":"unknown","speed":1}}`,
		`{"type":"scenario.start","params":{"scenario":"normal_drive","speed":3}}`,
		`{"type":"scenario.stop","params":{"unexpected":true}}`,
		`{"type":"camera.snapshot","params":{}}`,
		`{"type":"scenario.stop","params":{},"command_id":"client-id"}`,
		`{"type":"scenario.stop","params":{}} {}`,
	} {
		if _, err := ParseCommandRequest([]byte(body)); err == nil {
			t.Fatalf("accepted %s", body)
		}
	}
}

func TestCommandAndAckValidation(t *testing.T) {
	at := time.Now().UTC()
	cmd := Command{SchemaVersion: 1, DeviceID: "car-001", CommandID: "cmd-1", Type: "scenario.stop", Params: json.RawMessage(`{}`), IssuedAt: at, ExpiresAt: at.Add(10 * time.Second)}
	raw, _ := json.Marshal(cmd)
	if _, err := ParseCommand("car/car-001/commands", raw); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseCommand("car/car-002/commands", raw); err == nil {
		t.Fatal("accepted wrong device")
	}
	ack := CommandAck{SchemaVersion: 1, DeviceID: "car-001", CommandID: "cmd-1", Status: "success", ExecutedAt: at, Result: json.RawMessage(`{}`)}
	raw, _ = json.Marshal(ack)
	if _, err := ParseCommandAck("car/car-001/command-acks", raw); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*CommandAck){
		func(a *CommandAck) { a.SchemaVersion = 2 }, func(a *CommandAck) { a.CommandID = "" },
		func(a *CommandAck) { a.Status = "sent" }, func(a *CommandAck) { a.ExecutedAt = time.Time{} },
		func(a *CommandAck) { a.ExecutedAt = at.In(time.FixedZone("non-UTC", 3600)) },
		func(a *CommandAck) { a.Result = json.RawMessage(`null`) },
		func(a *CommandAck) { a.Error = &CommandError{Code: "ERROR", Message: "conflicting"} },
		func(a *CommandAck) {
			a.Status = "failed"
			a.Result = nil
			a.Error = &CommandError{Code: "", Message: "missing code"}
		},
	} {
		invalid := ack
		mutate(&invalid)
		raw, _ := json.Marshal(invalid)
		if _, err := ParseCommandAck("car/car-001/command-acks", raw); err == nil {
			t.Fatal("accepted invalid Ack", string(raw))
		}
	}
	raw, _ = json.Marshal(ack)
	if _, err := ParseCommandAck("car/car-002/command-acks", raw); err == nil {
		t.Fatal("accepted mismatched topic")
	}
	if _, err := ParseCommandAck("car/car-001/command-acks", []byte(strings.TrimSuffix(string(raw), "}")+`,"unexpected":true}`)); err == nil {
		t.Fatal("accepted extra field")
	}
}
