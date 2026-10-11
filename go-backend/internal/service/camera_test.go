package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"piguard/go-backend/internal/model"
	"piguard/go-backend/internal/protocol"
)

func TestCameraAckRejectsInvalidReferencesAndKeepsFirstTerminal(t *testing.T) {
	f := newCommandFixture(t)
	ctx := context.Background()
	if err := f.s.devices.Insert(ctx, &model.Device{DeviceID: "car-002", Name: "Other"}); err != nil {
		t.Fatal(err)
	}
	u := frameInput(t, "camera-good")
	u.Type = "snapshot"
	good, _, err := f.s.frames.Save(ctx, "car-001", u)
	if err != nil {
		t.Fatal(err)
	}
	u.FrameID = "camera-other"
	other, _, err := f.s.frames.Save(ctx, "car-002", u)
	if err != nil {
		t.Fatal(err)
	}
	u.FrameID = "camera-preview"
	u.Type = "preview"
	preview, _, err := f.s.frames.Save(ctx, "car-001", u)
	if err != nil {
		t.Fatal(err)
	}
	u.FrameID = "camera-event"
	u.Type = "event"
	event, _, err := f.s.frames.Save(ctx, "car-001", u)
	if err != nil {
		t.Fatal(err)
	}
	f.start(t)
	id, err := f.s.Create(ctx, "car-001", protocol.CommandRequest{Type: "camera.snapshot", Params: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	view := waitCommand(t, f, id, "sent")
	if string(view.Params) != `{"quality":80}` {
		t.Fatal("default quality not persisted")
	}
	sub := f.hub.Subscribe()
	defer f.hub.Unsubscribe(sub)
	for _, result := range []json.RawMessage{
		json.RawMessage(`{}`), json.RawMessage(`{"snapshot_id":"unknown"}`),
		json.RawMessage(`{"snapshot_id":null}`), json.RawMessage(`{"snapshot_id":"unknown","url":"fake"}`),
		json.RawMessage(`{"snapshot_id":"` + other.SnapshotID + `"}`),
		json.RawMessage(`{"snapshot_id":"` + preview.SnapshotID + `"}`),
		json.RawMessage(`{"snapshot_id":"` + event.SnapshotID + `"}`),
	} {
		a := successAck(id, time.Now().UTC())
		a.Result = result
		if err := ingestCommandAck(f.s, a); err == nil {
			t.Fatal("invalid camera result accepted", string(result))
		}
		if current, err := f.s.Get(ctx, id); err != nil || current.Status != "sent" {
			t.Fatal("invalid result ended command", err)
		}
		if len(sub.Messages) != 0 {
			t.Fatal("invalid Ack published command update")
		}
	}
	a := successAck(id, time.Now().UTC())
	a.Result = json.RawMessage(`{"snapshot_id":"` + good.SnapshotID + `"}`)
	if err := ingestCommandAck(f.s, a); err != nil {
		t.Fatal(err)
	}
	view = waitCommand(t, f, id, "success")
	var result struct {
		ID  string `json:"snapshot_id"`
		URL string `json:"url"`
	}
	if err := json.Unmarshal(view.Result, &result); err != nil || result.ID != good.SnapshotID || result.URL != protocol.SnapshotURL(good.SnapshotID) {
		t.Fatal("camera result missing stable URL", err)
	}
	for range 3 {
		if err := ingestCommandAck(f.s, a); err != nil {
			t.Fatal(err)
		}
	}
	a.Result = json.RawMessage(`{"snapshot_id":"conflict"}`)
	if err := ingestCommandAck(f.s, a); err != nil {
		t.Fatal(err)
	}
	if len(sub.Messages) != 1 {
		t.Fatal("duplicate/conflicting terminal Ack published again")
	}
	f.s.Close()
	restored := NewCommandService(f.r, f.s.devices, f.p, f.s.online, f.hub, f.s.frames)
	restored.now = f.s.now
	if err := restored.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	current, err := restored.Get(ctx, id)
	if err != nil || current.Status != "success" || string(current.Result) != string(view.Result) {
		t.Fatal("restart lost camera result", err)
	}
}

func TestCameraLateAckDoesNotInventSuccess(t *testing.T) {
	f := newCommandFixture(t)
	ctx := context.Background()
	f.start(t)
	id, err := f.s.Create(ctx, "car-001", protocol.CommandRequest{Type: "camera.snapshot", Params: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	view := waitCommand(t, f, id, "sent")
	f.clock.Store(view.ExpiresAt.UnixNano())
	a := successAck(id, time.Now().UTC())
	a.Result = json.RawMessage(`{"snapshot_id":"late-unknown"}`)
	if err := ingestCommandAck(f.s, a); err != nil {
		t.Fatal(err)
	}
	view = waitCommand(t, f, id, "timeout")
	if view.AckAt != nil || view.Result != nil {
		t.Fatal("late camera Ack invented a result")
	}
}
