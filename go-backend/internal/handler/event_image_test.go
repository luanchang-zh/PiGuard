package handler

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"piguard/go-backend/internal/model"
	"piguard/go-backend/internal/protocol"
	"piguard/go-backend/internal/repo"
	"piguard/go-backend/internal/service"
)

func TestEventImageInvalidReferencesStorageFailureAndRestart(t *testing.T) {
	f := newFrameFixture(t)
	ctx := context.Background()
	events := service.NewEventService(repo.NewEventRepository(f.g), f.devices, f.hub, f.frames)
	f.frames.SetEventNotifier(events.FrameSaved)
	at := time.Now().UTC().Format(time.RFC3339Nano)
	preview := f.upload(t, "car-001", "preview-ref", "preview", at, jpegFixture(t, 1), 201)
	other := f.upload(t, "car-002", "other-ref", "snapshot", at, jpegFixture(t, 1), 201)
	good := f.upload(t, "car-001", "good-ref", "event", at, jpegFixture(t, 1), 201)
	for _, tc := range []struct{ id, snapshot, state string }{{"preview", preview.SnapshotID, "invalid"}, {"other", other.SnapshotID, "invalid"}, {"unknown", "not-yet-uploaded", "pending"}, {"good", good.SnapshotID, "available"}} {
		if err := events.Ingest(ctx, "car/car-001/events", alarmRaw("evt-"+tc.id, "car-001", "obstacle_warning", "started", "warning", at, `{}`, map[string]any{"snapshot_id": tc.snapshot})); err != nil {
			t.Fatal(err)
		}
		items, err := events.List(ctx, "car-001", repo.EventQuery{})
		if err != nil {
			t.Fatal(err)
		}
		var item protocol.EventItem
		for _, candidate := range items {
			if candidate.EventID == "evt-"+tc.id {
				item = candidate
			}
		}
		if item.SnapshotState != tc.state || (item.SnapshotID != nil) != (tc.state == "available") {
			t.Fatal("invalid image state", item)
		}
	}
	row, err := f.snapshots.Find(ctx, good.SnapshotID)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(f.dir, row.Path)
	if err := os.Rename(path, path+"-saved"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := events.List(ctx, "car-001", repo.EventQuery{}); err == nil {
		t.Fatal("storage failure concealed by event list")
	}
	sub := f.hub.Subscribe()
	defer f.hub.Unsubscribe(sub)
	if err := events.Ingest(ctx, "car/car-001/events", alarmRaw("evt-storage-fault", "car-001", "obstacle_warning", "started", "warning", at, `{}`, map[string]any{"snapshot_id": good.SnapshotID})); err != nil {
		t.Fatal("image fault prevented alarm save", err)
	}
	var notification struct {
		Type string
		Data protocol.EventPush
	}
	if err := json.Unmarshal(<-sub.Messages, &notification); err != nil || notification.Data.SnapshotState != "unavailable" || notification.Data.SnapshotID != nil {
		t.Fatal("storage fault advertised available image", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path+"-saved", path); err != nil {
		t.Fatal(err)
	}
	f.close()
	f.open(t)
	events = service.NewEventService(repo.NewEventRepository(f.g), f.devices, f.hub, f.frames)
	items, err := events.List(ctx, "car-001", repo.EventQuery{})
	if err != nil || len(items) != 5 {
		t.Fatal("restart lost events", err)
	}
	for _, item := range items {
		if item.EventID == "evt-good" && (item.SnapshotState != "available" || item.SnapshotID == nil || *item.SnapshotID != good.SnapshotID) {
			t.Fatal("restart lost image association")
		}
	}
	// Verify the post-restart DTO through the actual HTTP handler as well.
	f.server.Close()
	f.server = httptest.NewServer(NewEngine(Dependencies{Auth: disabledTestAuth(t), Devices: service.NewDeviceService(f.devices, repo.NewConfigRepository(f.g)), Events: events, Frames: f.frames, Hub: f.hub}))
	response, err := f.server.Client().Get(f.server.URL + "/api/v1/devices/car-001/events")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatal("restored HTTP events unavailable")
	}
}

func TestConcurrentEventAndImageRetriesPublishOnce(t *testing.T) {
	f := newFrameFixture(t)
	events := service.NewEventService(repo.NewEventRepository(f.g), f.devices, f.hub, f.frames)
	f.frames.SetEventNotifier(events.FrameSaved)
	sub := f.hub.Subscribe()
	defer f.hub.Unsubscribe(sub)
	at := time.Now().UTC()
	raw := alarmRaw("evt-concurrent-image", "car-001", "obstacle_warning", "started", "warning", at.Format(time.RFC3339Nano), `{}`, map[string]any{"frame_id": "concurrent-frame"})
	u := protocol.FrameUpload{FrameID: "concurrent-frame", Type: "event", CapturedAt: at, JPEG: jpegFixture(t, 1)}
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			if err := events.Ingest(context.Background(), "car/car-001/events", raw); err != nil {
				t.Error(err)
			}
		}()
		go func() {
			defer wg.Done()
			if _, _, err := f.frames.Save(context.Background(), "car-001", u); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	for _, table := range []any{&model.Event{}, &model.Snapshot{}} {
		var count int64
		if err := f.g.Model(table).Count(&count).Error; err != nil || count != 1 {
			t.Fatal("concurrent retries duplicated records", count, err)
		}
	}
	items, err := events.List(context.Background(), "car-001", repo.EventQuery{})
	if err != nil || len(items) != 1 || items[0].SnapshotState != "available" {
		t.Fatal("concurrent image not linked", err)
	}
	kinds := map[string]int{}
	for len(sub.Messages) > 0 {
		var e struct{ Type string }
		json.Unmarshal(<-sub.Messages, &e)
		kinds[e.Type]++
	}
	if kinds["event"] != 1 || kinds["frame_update"] != 1 || kinds["event_image_update"] > 1 {
		t.Fatal("concurrent retries duplicated notifications", kinds)
	}
}
