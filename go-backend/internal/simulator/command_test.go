package simulator

import (
	"encoding/json"
	"piguard/go-backend/internal/protocol"
	"sync"
	"testing"
	"time"
)

func TestCommandExecutorDeduplicatesAndRejectsExpired(t *testing.T) {
	at := time.Now().UTC()
	for _, mode := range []string{"success", "failed", "none"} {
		t.Run(mode, func(t *testing.T) {
			e, err := NewCommandExecutor(mode)
			if err != nil {
				t.Fatal(err)
			}
			cmd := protocol.Command{SchemaVersion: 1, DeviceID: "car-001", CommandID: "same-id", Type: "scenario.stop", Params: json.RawMessage(`{}`), IssuedAt: at, ExpiresAt: at.Add(10 * time.Second)}
			raw, _ := json.Marshal(cmd)
			first, executed, err := e.Execute("car/car-001/commands", raw, at)
			if err != nil || !executed {
				t.Fatal(executed, err)
			}
			var wg sync.WaitGroup
			for i := 0; i < 20; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					ack, did, err := e.Execute("car/car-001/commands", raw, at.Add(time.Second))
					if err != nil || did || ack != first {
						t.Error("redelivery was not cached")
					}
				}()
			}
			wg.Wait()
			if e.Executions() != 1 {
				t.Fatal("duplicate execution", e.Executions())
			}
			if (mode == "none") != (first == nil) {
				t.Fatal("no-Ack mode incorrect")
			}
			cmd.CommandID = "expired-id"
			raw, _ = json.Marshal(cmd)
			ack, did, err := e.Execute("car/car-001/commands", raw, cmd.ExpiresAt)
			if err != nil || did || ack.Error == nil || ack.Error.Code != "COMMAND_EXPIRED" || e.Executions() != 1 {
				t.Fatal("expired command executed", ack, err)
			}
			if _, _, err := e.Execute("car/other/commands", raw, at); err == nil {
				t.Fatal("wrong identity accepted")
			}
		})
	}
	if _, err := NewCommandExecutor("bad"); err == nil {
		t.Fatal("accepted invalid mode")
	}
}
