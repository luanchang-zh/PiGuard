package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"piguard/go-backend/internal/apperr"
	"piguard/go-backend/internal/model"
	"piguard/go-backend/internal/protocol"
	"piguard/go-backend/internal/realtime"
	"piguard/go-backend/internal/repo"
	"time"
)

type FrameService interface {
	Save(context.Context, string, protocol.FrameUpload) (protocol.FrameResult, bool, error)
	Latest(context.Context, string) (protocol.FrameView, error)
	Open(context.Context, string) (*os.File, error)
}
type frameService struct {
	snapshots repo.SnapshotRepository
	devices   repo.DeviceRepository
	store     frameStore
	hub       *realtime.Hub
	now       func() time.Time
	// Only short DB acceptance and ordered publication are serialized.
	commit chan struct{}
}

func NewFrameService(snapshots repo.SnapshotRepository, devices repo.DeviceRepository, dir string, hub *realtime.Hub) FrameService {
	return &frameService{snapshots: snapshots, devices: devices, store: diskFrameStore{dir}, hub: hub, now: time.Now, commit: make(chan struct{}, 1)}
}
func (s *frameService) device(ctx context.Context, device string) error {
	_, err := s.devices.Find(ctx, device)
	if errors.Is(err, repo.ErrNotFound) {
		return apperr.ErrDeviceNotFound
	}
	return err
}
func frameResult(row *model.Snapshot) protocol.FrameResult {
	return protocol.FrameResult{SnapshotID: row.SnapshotID, DeviceID: row.DeviceID, Type: row.Type, CapturedAt: row.CapturedAt.UTC()}
}
func frameView(row *model.Snapshot) protocol.FrameView {
	return protocol.FrameView{SnapshotID: row.SnapshotID, Type: row.Type, CapturedAt: row.CapturedAt.UTC(), URL: "/api/v1/snapshots/" + row.SnapshotID + "/content"}
}
func (s *frameService) retry(ctx context.Context, row *model.Snapshot, u protocol.FrameUpload, digest string) (protocol.FrameResult, bool, error) {
	if row.Type != u.Type || !row.CapturedAt.Equal(u.CapturedAt) || row.Size != int64(len(u.JPEG)) || row.SHA256 != digest {
		return protocol.FrameResult{}, false, apperr.ErrFrameConflict
	}
	f, err := s.openRow(row)
	if err != nil {
		return protocol.FrameResult{}, false, err
	}
	defer f.Close()
	original, err := io.ReadAll(io.LimitReader(protocol.ContextReader{Context: ctx, Reader: f}, protocol.MaxJPEGBytes+1))
	if err != nil {
		return protocol.FrameResult{}, false, err
	}
	if !bytes.Equal(original, u.JPEG) {
		return protocol.FrameResult{}, false, apperr.ErrFrameConflict
	}
	return frameResult(row), false, nil
}
func (s *frameService) Save(ctx context.Context, device string, u protocol.FrameUpload) (protocol.FrameResult, bool, error) {
	if err := s.device(ctx, device); err != nil {
		return protocol.FrameResult{}, false, err
	}
	if err := u.Validate(ctx); err != nil {
		return protocol.FrameResult{}, false, err
	}
	u.CapturedAt = u.CapturedAt.UTC()
	sum := sha256.Sum256(u.JPEG)
	digest := hex.EncodeToString(sum[:])
	existing, err := s.snapshots.FindFrame(ctx, device, u.FrameID)
	if err == nil {
		return s.retry(ctx, existing, u, digest)
	}
	if !errors.Is(err, repo.ErrNotFound) {
		return protocol.FrameResult{}, false, err
	}
	var random [16]byte
	if _, err = rand.Read(random[:]); err != nil {
		return protocol.FrameResult{}, false, err
	}
	id := "snap-" + hex.EncodeToString(random[:])
	path, err := s.store.Save(ctx, id, u.JPEG)
	if err != nil {
		return protocol.FrameResult{}, false, err
	}
	committed := false
	defer func() {
		if !committed {
			if e := s.store.Remove(path); e != nil {
				slog.Error("回收未提交图片失败", "snapshot_id", id, "err", e)
			}
		}
	}()
	row := &model.Snapshot{SnapshotID: id, DeviceID: device, FrameID: &u.FrameID, Type: u.Type, Path: path, SHA256: digest, Size: int64(len(u.JPEG)), CapturedAt: u.CapturedAt, CapturedSeconds: u.CapturedAt.Unix(), CapturedNanosecond: u.CapturedAt.Nanosecond(), CreatedAt: s.now().UTC()}
	// Verify publication before entering the short acceptance transaction.
	f, err := s.openRow(row)
	if err != nil {
		return protocol.FrameResult{}, false, err
	}
	if err = f.Close(); err != nil {
		return protocol.FrameResult{}, false, err
	}
	select {
	case s.commit <- struct{}{}:
	case <-ctx.Done():
		return protocol.FrameResult{}, false, ctx.Err()
	}
	locked := true
	defer func() {
		if locked {
			<-s.commit
		}
	}()
	if err = ctx.Err(); err != nil {
		return protocol.FrameResult{}, false, err
	}
	accepted, created, latest, err := s.snapshots.Accept(ctx, row)
	if err != nil {
		return protocol.FrameResult{}, false, err
	}
	if !created {
		<-s.commit
		locked = false
		return s.retry(ctx, accepted, u, digest)
	}
	// Successful commit survives subsequent cancellation or a lost HTTP response.
	committed = true
	if s.hub != nil && (row.Type != "preview" || latest) {
		s.hub.Publish("frame_update", protocol.FramePush{FrameView: frameView(row), DeviceID: device}, s.now().UTC())
	}
	return frameResult(row), true, nil
}
func (s *frameService) Latest(ctx context.Context, device string) (protocol.FrameView, error) {
	if err := s.device(ctx, device); err != nil {
		return protocol.FrameView{}, err
	}
	row, err := s.snapshots.FindLatest(ctx, device)
	if errors.Is(err, repo.ErrNotFound) {
		return protocol.FrameView{}, apperr.ErrSnapshotNotFound
	}
	if err != nil {
		return protocol.FrameView{}, err
	}
	return frameView(row), nil
}
func (s *frameService) openRow(row *model.Snapshot) (*os.File, error) {
	if row.Path != row.SnapshotID+".jpg" {
		return nil, fmt.Errorf("invalid snapshot storage identity")
	}
	f, err := s.store.Open(row.Path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, apperr.ErrSnapshotNotFound
	}
	if err != nil {
		return nil, err
	}
	return f, nil
}
func (s *frameService) Open(ctx context.Context, id string) (*os.File, error) {
	row, err := s.snapshots.Find(ctx, id)
	if errors.Is(err, repo.ErrNotFound) {
		return nil, apperr.ErrSnapshotNotFound
	}
	if err != nil {
		return nil, err
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	return s.openRow(row)
}
