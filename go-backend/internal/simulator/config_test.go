package simulator

import (
	"encoding/json"
	"piguard/go-backend/internal/model"
	"piguard/go-backend/internal/protocol"
	"testing"
	"time"
)

func TestConfigExecutorModesRedeliveryConflictAndOldVersion(t *testing.T) {
	for _, mode := range []string{"success", "failed", "none"} {
		t.Run(mode, func(t *testing.T) {
			e, _ := NewConfigExecutor(mode)
			at := time.Now().UTC()
			m := protocol.ConfigMessage{SchemaVersion: 1, DeviceID: "car-001", ConfigVersion: 2, IssuedAt: at, Rules: model.DefaultRules().Rules}
			raw, _ := json.Marshal(m)
			ack, applied, err := e.Apply("car/car-001/config", raw, at)
			if err != nil || applied != (mode != "failed") || (ack == nil) != (mode == "none") {
				t.Fatal(ack, applied, err)
			}
			second, again, err := e.Apply("car/car-001/config", raw, at.Add(time.Hour))
			if err != nil || again || second != ack {
				t.Fatal("redelivery applied again")
			}
			m.Rules.Obstacle.WarningDistanceM = 20
			raw, _ = json.Marshal(m)
			if _, _, err = e.Apply("car/car-001/config", raw, at); err == nil {
				t.Fatal("accepted conflicting version")
			}
			m.ConfigVersion = 3
			raw, _ = json.Marshal(m)
			if _, _, err = e.Apply("car/car-001/config", raw, at); err != nil {
				t.Fatal(err)
			}
			m.ConfigVersion = 1
			raw, _ = json.Marshal(m)
			_, again, err = e.Apply("car/car-001/config", raw, at)
			if mode != "failed" && (err == nil || again || e.Version("car-001") != 3 || e.Applications() != 2) {
				t.Fatal("old config rolled back", err)
			}
			if mode == "failed" && (e.Version("car-001") != 0 || e.Applications() != 0) {
				t.Fatal("failed config applied")
			}
		})
	}
}
