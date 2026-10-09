package service

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"piguard/go-backend/internal/db"
	"piguard/go-backend/internal/model"
	"piguard/go-backend/internal/realtime"
	"piguard/go-backend/internal/repo"
	"piguard/go-backend/internal/testutil"
)

func TestEventDoesNotDeriveFromTelemetryOrChangeDevice(t *testing.T) {
	ctx := context.Background()
	g, err := db.Open(filepath.Join(t.TempDir(), "events.db"))
	if err != nil {
		t.Fatal(err)
	}
	sql, _ := g.DB()
	t.Cleanup(func() { sql.Close() })
	devices := repo.NewDeviceRepository(g)
	hub := realtime.NewHub(4)
	t.Cleanup(hub.Close)
	monitor := NewMonitor(devices, repo.NewTelemetryRepository(g), hub)
	if _, err := NewDeviceService(devices, repo.NewConfigRepository(g), monitor).Seed(ctx, SeedInput{DeviceID: "car-001", Name: "Demo", Mode: "mock", SoftwareVersion: "0.1.0"}); err != nil {
		t.Fatal(err)
	}
	sample := testutil.Telemetry("car-001", time.Now().UTC(), 1)
	sample.Risk.ActiveEvents = []string{"obstacle_warning"}
	if err := monitor.Ingest(ctx, "car/car-001/telemetry", testutil.JSON(sample)); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := g.Model(&model.Event{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatal(count, err)
	}
	before, err := devices.Find(ctx, "car-001")
	if err != nil || before.Online || before.LastSeenAt == nil {
		t.Fatal(before, err)
	}
	seen := *before.LastSeenAt
	events := NewEventService(repo.NewEventRepository(g), devices, hub)
	raw := []byte(`{"schema_version":1,"event_id":"evt-offline","device_id":"car-001","type":"sensor_failure","action":"started","level":"warning","timestamp":"2026-10-08T12:00:00Z","data":{"sensor":"distance","reason":"timeout"}}`)
	if err := events.Ingest(ctx, "car/missing/events", raw); err == nil {
		t.Fatal("accepted unknown device")
	}
	if err := events.Ingest(ctx, "car/car-001/events", raw); err != nil {
		t.Fatal(err)
	}
	after, err := devices.Find(ctx, "car-001")
	if err != nil || after.Online || after.LastSeenAt == nil || !after.LastSeenAt.Equal(seen) {
		t.Fatal(after, err)
	}
	if err := g.Model(&model.Event{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatal(count, err)
	}
}

func TestEventDuplicateInsertBroadcastsOnce(t *testing.T) {
	ctx := context.Background()
	g, err := db.Open(filepath.Join(t.TempDir(), "events.db"))
	if err != nil {
		t.Fatal(err)
	}
	sql, _ := g.DB()
	t.Cleanup(func() { sql.Close() })
	devices := repo.NewDeviceRepository(g)
	if err := devices.Insert(ctx, &model.Device{DeviceID: "car-001", Name: "Demo", Mode: "mock", SoftwareVersion: "0.1.0"}); err != nil {
		t.Fatal(err)
	}
	hub := realtime.NewHub(8)
	t.Cleanup(hub.Close)
	sub := hub.Subscribe()
	events := NewEventService(repo.NewEventRepository(g), devices, hub)
	raw := []byte(`{"schema_version":1,"event_id":"evt-race","device_id":"car-001","type":"sharp_turn","action":"level_changed","level":"danger","timestamp":"2026-10-08T12:00:00Z","data":{}}`)
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := events.Ingest(ctx, "car/car-001/events", raw); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	var count int64
	if err := g.Model(&model.Event{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatal(count, err)
	}
	select {
	case <-sub.Messages:
	default:
		t.Fatal("missing broadcast")
	}
	select {
	case <-sub.Messages:
		t.Fatal("duplicate broadcast")
	default:
	}
}

func TestEventListBreaksTimestampTiesByID(t *testing.T) {
	ctx := context.Background()
	g, err := db.Open(filepath.Join(t.TempDir(), "events.db"))
	if err != nil {
		t.Fatal(err)
	}
	sql, _ := g.DB()
	t.Cleanup(func() { sql.Close() })
	devices := repo.NewDeviceRepository(g)
	if err := devices.Insert(ctx, &model.Device{DeviceID: "car-001", Name: "Demo", Mode: "mock", SoftwareVersion: "0.1.0"}); err != nil {
		t.Fatal(err)
	}
	events := NewEventService(repo.NewEventRepository(g), devices, nil)
	for _, id := range []string{"evt-a", "evt-b"} {
		raw := []byte(`{"schema_version":1,"event_id":"` + id + `","device_id":"car-001","type":"sharp_turn","action":"started","level":"warning","timestamp":"2026-10-08T12:00:00Z","data":{}}`)
		if err := events.Ingest(ctx, "car/car-001/events", raw); err != nil {
			t.Fatal(err)
		}
	}
	items, err := events.List(ctx, "car-001", repo.EventQuery{})
	if err != nil || len(items) != 2 || items[0].EventID != "evt-b" || items[1].EventID != "evt-a" {
		t.Fatal(items, err)
	}
}

func TestEventInsertFailureDoesNotBroadcast(t *testing.T) {
	hub := realtime.NewHub(1)
	t.Cleanup(hub.Close)
	sub := hub.Subscribe()
	events := NewEventService(failEvents{}, deviceFinder{}, hub)
	err := events.Ingest(context.Background(), "car/car-001/events", []byte(`{"schema_version":1,"event_id":"evt-1","device_id":"car-001","type":"obstacle_warning","action":"started","level":"warning","timestamp":"2026-10-08T12:00:00Z","data":{}}`))
	if err == nil || err.Error() != "disk full" {
		t.Fatal(err)
	}
	select {
	case <-sub.Messages:
		t.Fatal("broadcast an unsaved event")
	default:
	}
}

func TestSlowEventClientDoesNotBlockIngest(t *testing.T) {
	ctx := context.Background()
	g, err := db.Open(filepath.Join(t.TempDir(), "events.db"))
	if err != nil {
		t.Fatal(err)
	}
	sql, _ := g.DB()
	t.Cleanup(func() { sql.Close() })
	devices := repo.NewDeviceRepository(g)
	if err := devices.Insert(ctx, &model.Device{DeviceID: "car-001", Name: "Demo", Mode: "mock", SoftwareVersion: "0.1.0"}); err != nil {
		t.Fatal(err)
	}
	hub := realtime.NewHub(1)
	t.Cleanup(hub.Close)
	slow, fast := hub.Subscribe(), hub.Subscribe()
	events := NewEventService(repo.NewEventRepository(g), devices, hub)
	base := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 20; i++ {
		id := "evt-" + base.Add(time.Duration(i)*time.Second).Format("150405")
		raw := []byte(`{"schema_version":1,"event_id":"` + id + `","device_id":"car-001","type":"lane_departure","action":"started","level":"warning","timestamp":"2026-10-08T12:00:00Z","data":{}}`)
		if err := events.Ingest(ctx, "car/car-001/events", raw); err != nil {
			t.Fatal(err)
		}
		select {
		case <-fast.Messages:
		case <-time.After(time.Second):
			t.Fatal("fast client blocked")
		}
	}
	select {
	case <-slow.Done:
	default:
		t.Fatal("slow client kept")
	}
}

type deviceFinder struct{}

func (deviceFinder) Find(context.Context, string) (*model.Device, error) {
	return &model.Device{DeviceID: "car-001"}, nil
}
func (deviceFinder) Insert(context.Context, *model.Device) error  { return nil }
func (deviceFinder) List(context.Context) ([]model.Device, error) { return nil, nil }
func (deviceFinder) Update(context.Context, string, map[string]any) error {
	return nil
}
func (deviceFinder) MarkAllOffline(context.Context) error { return nil }

type failEvents struct{}

func (failEvents) Insert(context.Context, *model.Event) error { return errors.New("disk full") }
func (failEvents) Find(context.Context, string) (*model.Event, error) {
	return nil, errors.New("unused")
}
func (failEvents) List(context.Context, string, repo.EventQuery) ([]model.Event, error) {
	return nil, nil
}
