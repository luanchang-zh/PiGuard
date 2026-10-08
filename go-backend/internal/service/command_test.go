package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"path/filepath"
	"piguard/go-backend/internal/apperr"
	"piguard/go-backend/internal/db"
	"piguard/go-backend/internal/model"
	"piguard/go-backend/internal/protocol"
	"piguard/go-backend/internal/realtime"
	"piguard/go-backend/internal/repo"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type commandPublisher struct {
	available atomic.Bool
	fn        func(context.Context, protocol.Command) error
	called    chan protocol.Command
}

func (p *commandPublisher) Available() bool { return p.available.Load() }
func (p *commandPublisher) Publish(ctx context.Context, topic string, qos byte, retained bool, raw []byte) error {
	cmd, err := protocol.ParseCommand(topic, raw)
	if err != nil {
		return err
	}
	if qos != 1 || retained {
		return errors.New("incorrect delivery options")
	}
	p.called <- *cmd
	if p.fn != nil {
		return p.fn(ctx, *cmd)
	}
	return nil
}

type commandFixture struct {
	s      *CommandManager
	r      repo.CommandRepository
	p      *commandPublisher
	hub    *realtime.Hub
	online atomic.Bool
	clock  atomic.Int64
}

func newCommandFixture(t *testing.T) *commandFixture {
	t.Helper()
	g, err := db.Open(filepath.Join(t.TempDir(), "commands.db"))
	if err != nil {
		t.Fatal(err)
	}
	sql, _ := g.DB()
	t.Cleanup(func() { sql.Close() })
	d := repo.NewDeviceRepository(g)
	if err := d.Insert(context.Background(), &model.Device{DeviceID: "car-001", Name: "Demo"}); err != nil {
		t.Fatal(err)
	}
	f := &commandFixture{r: repo.NewCommandRepository(g), p: &commandPublisher{called: make(chan protocol.Command, 512)}, hub: realtime.NewHub(128)}
	f.p.available.Store(true)
	f.online.Store(true)
	f.clock.Store(time.Now().UTC().UnixNano())
	f.s = NewCommandService(f.r, d, f.p, func(string) bool { return f.online.Load() }, f.hub)
	f.s.now = func() time.Time { return time.Unix(0, f.clock.Load()).UTC() }
	t.Cleanup(f.hub.Close)
	t.Cleanup(f.s.Close)
	return f
}
func (f *commandFixture) start(t *testing.T) {
	t.Helper()
	if err := f.s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
}
func commandRequest() protocol.CommandRequest {
	return protocol.CommandRequest{Type: "buzzer.test", Params: json.RawMessage(`{"duration_ms":1000}`)}
}
func createCommand(t *testing.T, f *commandFixture) string {
	t.Helper()
	id, err := f.s.Create(context.Background(), "car-001", commandRequest())
	if err != nil {
		t.Fatal(err)
	}
	return id
}
func waitCommand(t *testing.T, f *commandFixture, id, status string) *protocol.CommandView {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		view, err := f.s.Get(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		if view.Status == status {
			return view
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("command did not become", status)
	return nil
}
func successAck(id string, at time.Time) protocol.CommandAck {
	return protocol.CommandAck{SchemaVersion: 1, DeviceID: "car-001", CommandID: id, Status: "success", ExecutedAt: at, Result: json.RawMessage(`{"value":42}`)}
}
func ingestCommandAck(s *CommandManager, a protocol.CommandAck) error {
	raw, _ := json.Marshal(a)
	return s.IngestAck(context.Background(), "car/"+a.DeviceID+"/command-acks", raw)
}

func TestCommandFastAckTerminalIdempotency(t *testing.T) {
	f := newCommandFixture(t)
	sub := f.hub.Subscribe()
	f.p.fn = func(_ context.Context, c protocol.Command) error {
		return ingestCommandAck(f.s, successAck(c.CommandID, c.IssuedAt.Add(-time.Hour)))
	}
	f.start(t)
	id := createCommand(t, f)
	view := waitCommand(t, f, id, "success")
	if view.AckAt == nil || view.ExecutedAt == nil || !view.AckAt.After(*view.ExecutedAt) || view.ExpiresAt.Sub(view.IssuedAt) != 10*time.Second {
		t.Fatal("timestamps incorrect", view)
	}
	for _, status := range []string{"success", "failed"} {
		a := successAck(id, time.Now().UTC())
		a.Status = status
		if status == "failed" {
			a.Result = nil
			a.Error = &protocol.CommandError{Code: "DEVICE_BUSY", Message: "conflict"}
		}
		if err := ingestCommandAck(f.s, a); err != nil {
			t.Fatal(err)
		}
	}
	view = waitCommand(t, f, id, "success")
	if string(view.Result) != `{"value":42}` {
		t.Fatal("result overwritten")
	}
	statuses := []string{}
	for len(sub.Messages) > 0 {
		var event struct {
			Type string
			Data protocol.CommandView
		}
		if err := json.Unmarshal(<-sub.Messages, &event); err != nil {
			t.Fatal(err)
		}
		if event.Type != "command_update" {
			t.Fatal(event.Type)
		}
		statuses = append(statuses, event.Data.Status)
	}
	if len(statuses) != 2 || statuses[0] != "pending" || statuses[1] != "success" {
		t.Fatal("duplicate or regressing broadcasts", statuses)
	}
}

func TestCommandPublishResultsAndDeadline(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    error
		status string
	}{{"broker accepted", nil, "sent"}, {"refused", apperr.ErrMQTTUnavailable, "failed"}, {"uncertain", apperr.ErrPublishUncertain, "pending"}} {
		t.Run(tc.name, func(t *testing.T) {
			f := newCommandFixture(t)
			f.p.fn = func(context.Context, protocol.Command) error { return tc.err }
			f.start(t)
			id := createCommand(t, f)
			select {
			case <-f.p.called:
			case <-time.After(time.Second):
				t.Fatal("publish not called")
			}
			view := waitCommand(t, f, id, tc.status)
			if tc.status == "failed" {
				if len(view.Error) == 0 || view.AckAt != nil {
					t.Fatal("send failure not distinguished")
				}
				return
			}
			f.clock.Store(view.ExpiresAt.UnixNano())
			a := successAck(id, view.IssuedAt)
			if err := ingestCommandAck(f.s, a); err != nil {
				t.Fatal(err)
			}
			view = waitCommand(t, f, id, "timeout")
			if view.AckAt != nil || view.Result != nil {
				t.Fatal("late Ack altered timeout")
			}
		})
	}
}

func TestCommandRejectsWithoutPublishing(t *testing.T) {
	f := newCommandFixture(t)
	f.start(t)
	ctx := context.Background()
	if _, err := f.s.Create(ctx, "missing", commandRequest()); !errors.Is(err, apperr.ErrDeviceNotFound) {
		t.Fatal(err)
	}
	f.online.Store(false)
	if _, err := f.s.Create(ctx, "car-001", commandRequest()); !errors.Is(err, apperr.ErrDeviceOffline) {
		t.Fatal(err)
	}
	f.online.Store(true)
	f.p.available.Store(false)
	if _, err := f.s.Create(ctx, "car-001", commandRequest()); !errors.Is(err, apperr.ErrMQTTUnavailable) {
		t.Fatal(err)
	}
	f.p.available.Store(true)
	if _, err := f.s.Create(ctx, "car-001", protocol.CommandRequest{Type: "camera.snapshot", Params: json.RawMessage(`{}`)}); err == nil {
		t.Fatal("unsupported accepted")
	}
	if _, err := f.s.Get(ctx, "missing"); !errors.Is(err, apperr.ErrCommandNotFound) {
		t.Fatal(err)
	}
	if len(f.p.called) != 0 {
		t.Fatal("rejected request published")
	}
}

func TestCommandAckIdentityAndConcurrentOutcomes(t *testing.T) {
	f := newCommandFixture(t)
	f.start(t)
	id := createCommand(t, f)
	waitCommand(t, f, id, "sent")
	for _, change := range []func(*protocol.CommandAck){func(a *protocol.CommandAck) { a.DeviceID = "other" }, func(a *protocol.CommandAck) { a.CommandID = "unknown" }, func(a *protocol.CommandAck) { a.SchemaVersion = 2 }} {
		a := successAck(id, time.Now().UTC())
		change(&a)
		if err := ingestCommandAck(f.s, a); err == nil {
			t.Fatal("invalid Ack accepted")
		}
	}
	view, _ := f.s.Get(context.Background(), id)
	if view.Status != "sent" {
		t.Fatal(view.Status)
	}
	var wg sync.WaitGroup
	for _, status := range []string{"success", "failed"} {
		wg.Add(1)
		go func(status string) {
			defer wg.Done()
			a := successAck(id, time.Now().UTC())
			a.Status = status
			if status == "failed" {
				a.Result = nil
				a.Error = &protocol.CommandError{Code: "DEVICE_BUSY", Message: "busy"}
			}
			if err := ingestCommandAck(f.s, a); err != nil {
				t.Error(err)
			}
		}(status)
	}
	wg.Wait()
	view, _ = f.s.Get(context.Background(), id)
	if !((view.Status == "success" && view.Error == nil) || (view.Status == "failed" && view.Result == nil)) {
		t.Fatal("conflicting result", view)
	}
}

func TestCommandRecoveryNeverReplays(t *testing.T) {
	f := newCommandFixture(t)
	at := time.Unix(0, f.clock.Load()).UTC()
	ctx := context.Background()
	for _, tc := range []struct {
		id, status string
		expiry     time.Time
	}{{"expired-pending", "pending", at.Add(-time.Second)}, {"expired-sent", "sent", at.Add(-time.Second)}, {"pending", "pending", at.Add(10 * time.Second)}, {"sent", "sent", at.Add(10 * time.Second)}, {"done", "success", at.Add(-time.Second)}} {
		if err := f.r.Insert(ctx, &model.Command{CommandID: tc.id, DeviceID: "car-001", Type: "scenario.stop", Params: `{}`, Status: tc.status, IssuedAt: at.Add(-10 * time.Second), ExpiresAt: tc.expiry}); err != nil {
			t.Fatal(err)
		}
	}
	f.start(t)
	for _, id := range []string{"expired-pending", "expired-sent"} {
		waitCommand(t, f, id, "timeout")
	}
	waitCommand(t, f, "done", "success")
	if err := ingestCommandAck(f.s, successAck("pending", at)); err != nil {
		t.Fatal(err)
	}
	waitCommand(t, f, "pending", "success")
	f.s.Close()
	restored := NewCommandService(f.r, f.s.devices, f.p, f.s.online, f.hub)
	restored.now = f.s.now
	f.s = restored
	t.Cleanup(restored.Close)
	f.start(t)
	waitCommand(t, f, "pending", "success")
	waitCommand(t, f, "sent", "sent")
	f.clock.Store(at.Add(11 * time.Second).UnixNano())
	waitCommand(t, f, "sent", "timeout")
	if len(f.p.called) != 0 {
		t.Fatal("recovery replayed action")
	}
}

func TestCommandIgnoredAckDiagnostics(t *testing.T) {
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	defer slog.SetDefault(previous)
	for _, tc := range []struct {
		name, storedStatus, ackStatus, result, errorCode, reason string
		changedExecution                                         bool
	}{
		{"duplicate success", "success", "success", `{"b":2,"a":1}`, "", "duplicate", false},
		{"conflicting result", "success", "success", `{"a":2,"b":2}`, "", "conflict", false},
		{"conflicting execution", "success", "success", `{"a":1,"b":2}`, "", "conflict", true},
		{"conflicting status", "success", "failed", "", "DEVICE_BUSY", "conflict", false},
		{"duplicate failure", "failed", "failed", "", "DEVICE_BUSY", "duplicate", false},
		{"conflicting error", "failed", "failed", "", "INTERNAL_ERROR", "conflict", false},
		{"late after timeout", "timeout", "success", `{"a":1,"b":2}`, "", "late", false},
		{"late before scan", "sent", "success", `{"a":1,"b":2}`, "", "late", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newCommandFixture(t)
			at := time.Unix(0, f.clock.Load()).UTC()
			executed := at.Add(-time.Second)
			row := &model.Command{CommandID: "cmd-diagnostic", DeviceID: "car-001", Type: "scenario.stop", Params: `{}`, Status: tc.storedStatus, IssuedAt: at.Add(-10 * time.Second), ExpiresAt: at}
			if tc.storedStatus == "success" || tc.storedStatus == "failed" {
				row.ExecutedAt, row.AckAt = &executed, &executed
				if tc.storedStatus == "success" {
					row.Result = `{"a":1,"b":2}`
				} else {
					row.Error = `{"code":"DEVICE_BUSY","message":"busy"}`
				}
			}
			if err := f.r.Insert(context.Background(), row); err != nil {
				t.Fatal(err)
			}
			sub := f.hub.Subscribe()
			ack := successAck(row.CommandID, executed)
			ack.Status, ack.Result = tc.ackStatus, json.RawMessage(tc.result)
			if tc.errorCode != "" {
				ack.Error = &protocol.CommandError{Code: tc.errorCode, Message: "busy"}
			}
			if tc.changedExecution {
				ack.ExecutedAt = executed.Add(time.Millisecond)
			}
			logs.Reset()
			if err := ingestCommandAck(f.s, ack); err != nil {
				t.Fatal(err)
			}
			var entry struct {
				Reason     string    `json:"reason"`
				CommandID  string    `json:"command_id"`
				AckStatus  string    `json:"ack_status"`
				ExecutedAt time.Time `json:"executed_at"`
			}
			if err := json.Unmarshal(logs.Bytes(), &entry); err != nil {
				t.Fatal(err)
			}
			if entry.Reason != tc.reason || entry.CommandID != row.CommandID || entry.AckStatus != ack.Status || !entry.ExecutedAt.Equal(ack.ExecutedAt) {
				t.Fatal("missing or incorrect Ack diagnostic", entry)
			}
			stored, err := f.r.Find(context.Background(), row.CommandID)
			if err != nil {
				t.Fatal(err)
			}
			wantStatus, wantUpdates := row.Status, 0
			if tc.storedStatus == "sent" {
				wantStatus, wantUpdates = "timeout", 1
			}
			if stored.Status != wantStatus || stored.Result != row.Result || stored.Error != row.Error || len(sub.Messages) != wantUpdates {
				t.Fatal("ignored Ack changed persisted facts or broadcast count", stored, len(sub.Messages))
			}
		})
	}
}
