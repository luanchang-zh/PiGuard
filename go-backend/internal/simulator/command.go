// Package simulator implements the software device used by the local MQTT client.
package simulator

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"piguard/go-backend/internal/protocol"
)

type CommandExecutor struct {
	mu         sync.Mutex
	mode       string
	results    map[string]*protocol.CommandAck
	executions int
	capture    func(context.Context, protocol.Command, time.Time) (json.RawMessage, error)
}

func (e *CommandExecutor) SetCameraCapture(capture func(context.Context, protocol.Command, time.Time) (json.RawMessage, error)) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.capture = capture
}

func NewCommandExecutor(mode string) (*CommandExecutor, error) {
	if mode != "success" && mode != "failed" && mode != "none" {
		return nil, fmt.Errorf("ack-mode must be success, failed or none")
	}
	return &CommandExecutor{mode: mode, results: map[string]*protocol.CommandAck{}}, nil
}

// Execute caches even suppressed responses, so redelivery never executes twice.
func (e *CommandExecutor) Execute(topic string, raw []byte, at time.Time) (*protocol.CommandAck, bool, error) {
	return e.ExecuteContext(context.Background(), topic, raw, at)
}

func (e *CommandExecutor) ExecuteContext(ctx context.Context, topic string, raw []byte, at time.Time) (*protocol.CommandAck, bool, error) {
	cmd, err := protocol.ParseCommand(topic, raw)
	if err != nil {
		return nil, false, err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if ack, ok := e.results[cmd.CommandID]; ok {
		return ack, false, nil
	}
	ack := &protocol.CommandAck{SchemaVersion: 1, CommandID: cmd.CommandID, DeviceID: cmd.DeviceID, ExecutedAt: at.UTC()}
	if !at.Before(cmd.ExpiresAt) {
		ack.Status = "failed"
		ack.Error = &protocol.CommandError{Code: "COMMAND_EXPIRED", Message: "command expired before execution"}
		e.results[cmd.CommandID] = ack
		return ack, false, nil
	}
	e.executions++
	switch e.mode {
	case "none":
		ack = nil
	case "failed":
		ack.Status = "failed"
		ack.Error = &protocol.CommandError{Code: "ACTUATOR_UNAVAILABLE", Message: "simulated actuator failure"}
	case "success":
		ack.Status = "success"
		ack.Result = json.RawMessage(`{"message":"simulated command completed"}`)
		if cmd.Type == "camera.snapshot" {
			if e.capture == nil {
				ack.Status, ack.Result = "failed", nil
				ack.Error = &protocol.CommandError{Code: "DEVICE_NOT_SUPPORTED", Message: "snapshot uploader is not configured"}
			} else {
				result, err := e.capture(ctx, *cmd, at.UTC())
				if err != nil {
					ack.Status, ack.Result = "failed", nil
					ack.Error = &protocol.CommandError{Code: "SNAPSHOT_UPLOAD_FAILED", Message: err.Error()}
				} else {
					ack.Result = result
				}
			}
		}
	}
	e.results[cmd.CommandID] = ack
	return ack, true, nil
}

func (e *CommandExecutor) Executions() int { e.mu.Lock(); defer e.mu.Unlock(); return e.executions }
