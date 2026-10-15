package service

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"piguard/go-backend/internal/apperr"
	"piguard/go-backend/internal/db"
	"piguard/go-backend/internal/model"
	"piguard/go-backend/internal/protocol"
	"piguard/go-backend/internal/realtime"
	"piguard/go-backend/internal/repo"
	"piguard/go-backend/internal/testutil"
	"sync"
	"testing"
	"time"
)

func testMonitor(t *testing.T) (*Monitor, repo.DeviceRepository, repo.TelemetryRepository, DeviceService) {
	t.Helper()
	database, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	sql, _ := database.DB()
	t.Cleanup(func() { _ = sql.Close() })
	d, h := repo.NewDeviceRepository(database), repo.NewTelemetryRepository(database)
	hub := realtime.NewHub(128)
	t.Cleanup(hub.Close)
	m := NewMonitor(d, h, hub)
	svc := NewDeviceService(d, repo.NewConfigRepository(database), m)
	for _, id := range []string{"car-001", "car-002"} {
		if _, err := svc.Seed(context.Background(), SeedInput{DeviceID: id, Name: id, Mode: "mock", SoftwareVersion: "0.1.0"}); err != nil {
			t.Fatal(err)
		}
	}
	return m, d, h, svc
}

func TestMonitorOrderingHistoryFreshnessAndRecovery(t *testing.T) {
	m, d, h, svc := testMonitor(t)
	ctx := context.Background()
	base := time.Now().UTC()
	now := base
	m.now = func() time.Time { return now }
	empty, err := m.State(ctx, "car-001")
	if err != nil || empty.Timestamp != nil {
		t.Fatalf("bad initial state: %+v %v", empty, err)
	}
	if _, err := m.State(ctx, "unknown"); !errors.Is(err, apperr.ErrDeviceNotFound) {
		t.Fatal(err)
	}
	if err := m.Ingest(ctx, "car/car-001/status", testutil.JSON(testutil.Status("car-001", base, true))); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		now = base.Add(time.Duration(i) * 500 * time.Millisecond)
		for _, id := range []string{"car-001", "car-002"} {
			if err := m.Ingest(ctx, "car/"+id+"/telemetry", testutil.JSON(testutil.Telemetry(id, now, uint64(i)))); err != nil {
				t.Fatal(err)
			}
		}
	}
	state, _ := m.State(ctx, "car-001")
	if !state.Online || *state.Distance != 18 || state.Timestamp.Sub(base) != 9500*time.Millisecond {
		t.Fatalf("state incorrect: %+v", state)
	}
	rows, err := h.List(ctx, "car-001", repo.TelemetryQuery{Limit: 100})
	if err != nil || len(rows) != 10 {
		t.Fatalf("history: %d %v", len(rows), err)
	}
	for i, r := range rows {
		if i > 0 && r.ReceivedAt.Sub(rows[i-1].ReceivedAt) < time.Second {
			t.Fatal("write rate exceeded")
		}
	}
	rows2, _ := h.List(ctx, "car-002", repo.TelemetryQuery{Limit: 100})
	if len(rows2) != 10 {
		t.Fatal("per-device limit shared")
	}
	for _, msg := range [][]byte{testutil.JSON(testutil.Telemetry("car-001", base, 99)), testutil.JSON(testutil.Telemetry("car-001", now, 19))} {
		if err := m.Ingest(ctx, "car/car-001/telemetry", msg); err != nil {
			t.Fatal(err)
		}
	}
	unchanged, _ := m.State(ctx, "car-001")
	if !state.Timestamp.Equal(*unchanged.Timestamp) {
		t.Fatal("state regressed")
	}
	now = now.Add(time.Second)
	restart := testutil.Telemetry("car-001", now, 0)
	if err := m.Ingest(ctx, "car/car-001/telemetry", testutil.JSON(restart)); err != nil {
		t.Fatal(err)
	}
	state, _ = m.State(ctx, "car-001")
	if !state.Timestamp.Equal(now) {
		t.Fatal("seq restart rejected")
	}
	sub := m.hub.Subscribe()
	defer m.hub.Unsubscribe(sub)
	now = now.Add(2 * time.Second)
	m.Expire()
	select {
	case raw := <-sub.Messages:
		var event realtime.Event
		if err := json.Unmarshal(raw, &event); err != nil || event.Type != "telemetry" {
			t.Fatal("missing expiry event")
		}
	default:
		t.Fatal("no expiry event")
	}
	state, _ = m.State(ctx, "car-001")
	if state.Freshness["distance"] != "stale" || state.Freshness["temperature"] != "fresh" {
		t.Fatal(state.Freshness)
	}
	if err := m.Ingest(ctx, "car/car-001/status", testutil.JSON(testutil.Status("car-001", now, false))); err != nil {
		t.Fatal(err)
	}
	device, _ := d.Find(ctx, "car-001")
	if device.Online || device.SoftwareVersion != "0.2.0" || device.ReportedConfigVersion != 5 {
		t.Fatalf("will erased metadata: %+v", device)
	}
	fault := testutil.Telemetry("car-001", now.Add(time.Second), 1)
	fault.Temperature.Status = "error"
	fault.Speed.Value = nil
	if err := m.Ingest(ctx, "car/car-001/telemetry", testutil.JSON(fault)); err != nil {
		t.Fatal(err)
	}
	state, _ = m.State(ctx, "car-001")
	if state.Online || state.Temperature != nil || state.Speed != nil || state.Freshness["temperature"] != "fault" {
		t.Fatal("fault or offline lost")
	}
	before, _ := d.List(ctx)
	if err := m.Ingest(ctx, "car/unknown/telemetry", testutil.JSON(testutil.Telemetry("unknown", now, 1))); !errors.Is(err, apperr.ErrDeviceNotFound) {
		t.Fatal(err)
	}
	after, _ := d.List(ctx)
	if len(before) != len(after) {
		t.Fatal("unknown device registered")
	}
	if _, err := svc.Seed(ctx, SeedInput{DeviceID: "car-001", Name: "reset", Mode: "hardware", SoftwareVersion: "reset"}); err != nil {
		t.Fatal(err)
	}
	config, _ := svc.(*deviceService).configs.Find(ctx, "car-001")
	if config.DesiredVersion != 1 || config.ReportedVersion != 0 {
		t.Fatal("status fabricated config ack")
	}
	if err := m.Disconnected(ctx); err != nil {
		t.Fatal(err)
	}
	recovered := NewMonitor(d, h, realtime.NewHub(1))
	empty, err = recovered.State(ctx, "car-001")
	if err != nil || empty.Timestamp != nil || empty.Online {
		t.Fatal("restart replayed realtime state")
	}
	retained, _ := h.List(ctx, "car-001", repo.TelemetryQuery{Limit: 100})
	if len(retained) < 10 {
		t.Fatal("history lost")
	}
	// A restarted platform must not duplicate a previously persisted snapshot.
	recovered.now = m.now
	last, err := h.Latest(ctx, "car-001")
	if err != nil {
		t.Fatal(err)
	}
	var replay protocol.Telemetry
	if err := json.Unmarshal([]byte(last.PayloadJSON), &replay); err != nil {
		t.Fatal(err)
	}
	if err := recovered.Ingest(ctx, "car/car-001/telemetry", testutil.JSON(replay)); err != nil {
		t.Fatal(err)
	}
	retainedAfter, _ := h.List(ctx, "car-001", repo.TelemetryQuery{Limit: 100})
	if len(retainedAfter) != len(retained) {
		t.Fatal("restart duplicated old snapshot")
	}
}

func TestConcurrentMonitoring(t *testing.T) {
	m, _, _, _ := testMonitor(t)
	ctx := context.Background()
	base := time.Now().UTC()
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = m.Ingest(ctx, "car/car-001/telemetry", testutil.JSON(testutil.Telemetry("car-001", base.Add(time.Duration(i)*time.Millisecond), uint64(i))))
			_, _ = m.State(ctx, "car-001")
			m.Expire()
		}(i)
	}
	wg.Wait()
	state, _ := m.State(ctx, "car-001")
	if !state.Timestamp.Equal(base.Add(29 * time.Millisecond)) {
		t.Fatal("concurrent state regressed")
	}
}

type failedHistory struct{ repo.TelemetryRepository }

func (f failedHistory) Insert(context.Context, *model.Telemetry) error {
	return errors.New("disk unavailable")
}

func TestHistoryFailureDoesNotLoseRealtime(t *testing.T) {
	m, _, h, _ := testMonitor(t)
	m.history = failedHistory{h}
	at := time.Now()
	if err := m.Ingest(context.Background(), "car/car-001/telemetry", testutil.JSON(testutil.Telemetry("car-001", at, 1))); err == nil {
		t.Fatal("failed write reported success")
	}
	state, err := m.State(context.Background(), "car-001")
	if err != nil || state.Timestamp == nil {
		t.Fatal("history failure destroyed realtime")
	}
}
