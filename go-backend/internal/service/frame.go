package service

import (
	"context"
	"fmt"

	"piguard/go-backend/internal/apperr"
	"piguard/go-backend/internal/repo"
)

// FrameService 负责接收树莓派上传的 JPEG，并提供给网页读取。
// 文件会写到启动时准备好的截图目录，数据库只记路径。
type FrameService interface {
	Save(ctx context.Context, deviceID string) error
	Latest(ctx context.Context, deviceID string) error
	Open(ctx context.Context, snapshotID string) error
}

type frameService struct {
	snapshots repo.SnapshotRepository
	dir       string
}

func NewFrameService(snapshots repo.SnapshotRepository, dir string) FrameService {
	return &frameService{snapshots: snapshots, dir: dir}
}

func (s *frameService) Save(context.Context, string) error {
	if err := s.ready(); err != nil {
		return err
	}
	return apperr.ErrNotImplemented
}

func (s *frameService) Latest(ctx context.Context, deviceID string) error {
	if err := s.ready(); err != nil {
		return err
	}
	_, err := s.snapshots.FindLatest(ctx, deviceID)
	return err
}

func (s *frameService) Open(ctx context.Context, snapshotID string) error {
	if err := s.ready(); err != nil {
		return err
	}
	_, err := s.snapshots.Find(ctx, snapshotID)
	return err
}

func (s *frameService) ready() error {
	if s.snapshots == nil || s.dir == "" {
		return fmt.Errorf("图片依赖未注入")
	}
	return nil
}
