package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"gorm.io/gorm"
	"image"
	"image/jpeg"
	"io"
	"os"
	"path/filepath"
	"piguard/go-backend/internal/db"
	"piguard/go-backend/internal/model"
	"piguard/go-backend/internal/protocol"
	"piguard/go-backend/internal/realtime"
	"piguard/go-backend/internal/repo"
	"testing"
	"time"
)

type frameServiceFixture struct {
	s   *frameService
	g   *gorm.DB
	dir string
	hub *realtime.Hub
}

func newFrameServiceFixture(t *testing.T) *frameServiceFixture {
	t.Helper()
	g, err := db.Open(filepath.Join(t.TempDir(), "frames.db"))
	if err != nil {
		t.Fatal(err)
	}
	sql, _ := g.DB()
	t.Cleanup(func() { sql.Close() })
	devices := repo.NewDeviceRepository(g)
	if err = devices.Insert(context.Background(), &model.Device{DeviceID: "car-001", Name: "demo", Mode: "mock", SoftwareVersion: "0.1.0"}); err != nil {
		t.Fatal(err)
	}
	f := &frameServiceFixture{g: g, dir: t.TempDir(), hub: realtime.NewHub(1)}
	t.Cleanup(f.hub.Close)
	f.s = NewFrameService(repo.NewSnapshotRepository(g), devices, f.dir, f.hub).(*frameService)
	return f
}
func frameInput(t *testing.T, id string) protocol.FrameUpload {
	t.Helper()
	var b bytes.Buffer
	if err := jpeg.Encode(&b, image.NewGray(image.Rect(0, 0, 8, 8)), nil); err != nil {
		t.Fatal(err)
	}
	return protocol.FrameUpload{FrameID: id, Type: "preview", CapturedAt: time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC), JPEG: b.Bytes()}
}

type frameRepoHook struct {
	repo.SnapshotRepository
	accept func(context.Context, *model.Snapshot) (*model.Snapshot, bool, bool, error)
}

func (r frameRepoHook) Accept(ctx context.Context, row *model.Snapshot) (*model.Snapshot, bool, bool, error) {
	return r.accept(ctx, row)
}

type frameStoreHook struct {
	frameStore
	save func(context.Context, string, []byte) (string, error)
	open func(string) (*os.File, error)
}

func (s frameStoreHook) Save(ctx context.Context, id string, raw []byte) (string, error) {
	if s.save != nil {
		return s.save(ctx, id, raw)
	}
	return s.frameStore.Save(ctx, id, raw)
}
func (s frameStoreHook) Open(path string) (*os.File, error) {
	if s.open != nil {
		return s.open(path)
	}
	return s.frameStore.Open(path)
}
func assertNoFrame(t *testing.T, f *frameServiceFixture, sub *realtime.Subscription) {
	t.Helper()
	var count int64
	if err := f.g.Model(&model.Snapshot{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(f.dir)
	if err != nil || count != 0 || len(entries) != 0 || len(sub.Messages) != 0 {
		t.Fatal(count, len(entries), len(sub.Messages), err)
	}
	if _, err = f.s.Latest(context.Background(), "car-001"); err == nil {
		t.Fatal("failed upload advanced preview")
	}
}
func TestFrameFailuresAndCancellationCommitBoundary(t *testing.T) {
	for _, stage := range []string{"before-validation", "after-file", "before-commit", "commit-error", "published-unreadable", "waiting-commit", "after-commit"} {
		t.Run(stage, func(t *testing.T) {
			f := newFrameServiceFixture(t)
			sub := f.hub.Subscribe()
			defer f.hub.Unsubscribe(sub)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			originalRepo, originalStore := f.s.snapshots, f.s.store
			switch stage {
			case "before-validation":
				cancel()
			case "after-file":
				f.s.store = frameStoreHook{frameStore: originalStore, save: func(ctx context.Context, id string, raw []byte) (string, error) {
					path, err := originalStore.Save(ctx, id, raw)
					cancel()
					return path, err
				}}
			case "before-commit":
				f.s.snapshots = frameRepoHook{SnapshotRepository: originalRepo, accept: func(ctx context.Context, row *model.Snapshot) (*model.Snapshot, bool, bool, error) {
					cancel()
					return originalRepo.Accept(ctx, row)
				}}
			case "commit-error":
				f.s.snapshots = frameRepoHook{SnapshotRepository: originalRepo, accept: func(ctx context.Context, row *model.Snapshot) (*model.Snapshot, bool, bool, error) {
					return nil, false, false, errors.New("injected commit failure")
				}}
			case "published-unreadable":
				f.s.store = frameStoreHook{frameStore: originalStore, open: func(string) (*os.File, error) { return nil, errors.New("injected read failure") }}
			case "waiting-commit":
				f.s.commit <- struct{}{}
				defer func() { <-f.s.commit }()
				f.s.store = frameStoreHook{frameStore: originalStore, save: func(ctx context.Context, id string, raw []byte) (string, error) {
					path, err := originalStore.Save(ctx, id, raw)
					cancel()
					return path, err
				}}
			case "after-commit":
				f.s.snapshots = frameRepoHook{SnapshotRepository: originalRepo, accept: func(ctx context.Context, row *model.Snapshot) (*model.Snapshot, bool, bool, error) {
					r, c, l, err := originalRepo.Accept(ctx, row)
					if err == nil {
						cancel()
					}
					return r, c, l, err
				}}
			}
			input := frameInput(t, "f-1")
			data, created, err := f.s.Save(ctx, "car-001", input)
			if stage != "after-commit" {
				if err == nil || created {
					t.Fatal(data, created, err)
				}
				assertNoFrame(t, f, sub)
				return
			}
			if err != nil || !created || ctx.Err() == nil || len(sub.Messages) != 1 {
				t.Fatal(data, created, err, len(sub.Messages))
			}
			var env struct {
				Data protocol.FramePush `json:"data"`
			}
			_ = json.Unmarshal(<-sub.Messages, &env)
			if env.Data.SnapshotID != data.SnapshotID {
				t.Fatal(env, data)
			}
			file, err := f.s.Open(context.Background(), data.SnapshotID)
			if err != nil {
				t.Fatal(err)
			}
			raw, err := io.ReadAll(file)
			file.Close()
			if err != nil || !bytes.Equal(raw, input.JPEG) {
				t.Fatal(err)
			}
			retry, created, err := f.s.Save(context.Background(), "car-001", input)
			if err != nil || created || retry.SnapshotID != data.SnapshotID || len(sub.Messages) != 0 {
				t.Fatal(retry, created, err)
			}
		})
	}
}
func TestFrameDiskPublicationNeverOverwritesAndConfinesReads(t *testing.T) {
	dir := t.TempDir()
	store := diskFrameStore{dir}
	ctx := context.Background()
	raw := frameInput(t, "f").JPEG
	path, err := store.Save(ctx, "snap-test", raw)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.Save(ctx, "snap-test", []byte("overwrite")); err == nil {
		t.Fatal("overwrote immutable file")
	}
	saved, err := os.ReadFile(filepath.Join(dir, path))
	if err != nil || !bytes.Equal(saved, raw) {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatal("publication failure left temp", entries)
	}
	if err = os.WriteFile(filepath.Join(dir, ".pending-snap-collision"), []byte("user data"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Save(ctx, "snap-collision", raw); err == nil {
		t.Fatal("overwrote user temp")
	}
	saved, _ = os.ReadFile(filepath.Join(dir, ".pending-snap-collision"))
	if string(saved) != "user data" {
		t.Fatal("deleted another file")
	}
	for _, path := range []string{"../secret", "/etc/passwd", "a/../../secret"} {
		if f, err := store.Open(path); err == nil {
			f.Close()
			t.Fatal("escaped root", path)
		}
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err = store.Save(canceled, "snap-canceled", raw); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	entries, _ = os.ReadDir(dir)
	if len(entries) != 2 {
		t.Fatal("cancel residue", entries)
	}
}
func TestFrameSlowSubscriberDoesNotBlockAndWideTimestampOrder(t *testing.T) {
	f := newFrameServiceFixture(t)
	ctx := context.Background()
	slow, fast := f.hub.Subscribe(), f.hub.Subscribe()
	defer f.hub.Unsubscribe(slow)
	defer f.hub.Unsubscribe(fast)
	first := frameInput(t, "future")
	first.CapturedAt = time.Date(9999, 12, 31, 23, 59, 59, 999999999, time.UTC)
	a, _, err := f.s.Save(ctx, "car-001", first)
	if err != nil {
		t.Fatal(err)
	}
	<-fast.Messages
	// UnixNano overflows beyond 2262; ordering must still keep this future preview.
	old := frameInput(t, "past")
	old.CapturedAt = time.Date(1, 1, 1, 0, 0, 0, 0, time.UTC)
	_, _, err = f.s.Save(ctx, "car-001", old)
	if err != nil {
		t.Fatal(err)
	}
	current, err := f.s.Latest(ctx, "car-001")
	if err != nil || current.SnapshotID != a.SnapshotID || len(fast.Messages) != 0 {
		t.Fatal(current, err)
	}
	next := frameInput(t, "event")
	next.Type = "event"
	start := time.Now()
	_, _, err = f.s.Save(ctx, "car-001", next)
	if err != nil || time.Since(start) > time.Second {
		t.Fatal(err)
	}
	select {
	case <-slow.Done:
	default:
		t.Fatal("slow queue was not closed")
	}
	if len(fast.Messages) != 1 {
		t.Fatal("fast subscriber blocked")
	}
}
