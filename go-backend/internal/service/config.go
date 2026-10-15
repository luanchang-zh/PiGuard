package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

	"piguard/go-backend/internal/apperr"
	"piguard/go-backend/internal/model"
	"piguard/go-backend/internal/protocol"
	"piguard/go-backend/internal/repo"
)

type ConfigService interface {
	Get(context.Context, string) (*protocol.ConfigView, error)
	Update(context.Context, string, protocol.ConfigRequest) (int, error)
}

// One sender serializes retained writes. Pending IDs are coalesced and bounded
// by registered devices; network waits never hold this mutex or a DB transaction.
type ConfigManager struct {
	mu      sync.Mutex
	configs repo.ConfigRepository
	devices repo.DeviceRepository
	pub     Publisher
	now     func() time.Time
	ctx     context.Context
	cancel  context.CancelFunc
	running bool
	workers sync.WaitGroup
	pending map[string]struct{}
	wake    chan struct{}
}

func NewConfigService(configs repo.ConfigRepository, pub Publisher, devices repo.DeviceRepository) *ConfigManager {
	return &ConfigManager{configs: configs, pub: pub, devices: devices, now: time.Now, pending: map[string]struct{}{}, wake: make(chan struct{}, 1)}
}
func (s *ConfigManager) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ctx != nil {
		return fmt.Errorf("config manager already started")
	}
	if err := s.configs.EnsureRevisions(ctx); err != nil {
		return err
	}
	s.ctx, s.cancel = context.WithCancel(ctx)
	s.running = true
	s.workers.Add(1)
	go s.sendLoop()
	return nil
}
func (s *ConfigManager) Close() {
	s.mu.Lock()
	s.running = false
	if s.cancel != nil {
		s.cancel()
	}
	s.mu.Unlock()
	s.workers.Wait()
}
func (s *ConfigManager) requireDevice(ctx context.Context, id string) error {
	_, err := s.devices.Find(ctx, id)
	if errors.Is(err, repo.ErrNotFound) {
		return apperr.ErrDeviceNotFound
	}
	return err
}
func (s *ConfigManager) Get(ctx context.Context, id string) (*protocol.ConfigView, error) {
	if err := s.requireDevice(ctx, id); err != nil {
		return nil, err
	}
	row, err := s.configs.Snapshot(ctx, id)
	if err != nil {
		return nil, err
	}
	var document model.RulesConfig
	if err = json.Unmarshal([]byte(row.Config.ConfigJSON), &document); err != nil {
		return nil, err
	}
	v := &protocol.ConfigView{DesiredVersion: row.Config.DesiredVersion, ReportedVersion: row.Config.ReportedVersion, Rules: document.Rules, Status: row.Revision.Status, AckAt: row.Revision.AckAt, AppliedAt: row.Revision.AppliedAt}
	if row.Revision.ErrorJSON != "" {
		if err = json.Unmarshal([]byte(row.Revision.ErrorJSON), &v.Error); err != nil {
			return nil, err
		}
	}
	return v, nil
}
func (s *ConfigManager) enqueueLocked(id string) {
	s.pending[id] = struct{}{}
	select {
	case s.wake <- struct{}{}:
	default:
	}
}
func (s *ConfigManager) Update(ctx context.Context, id string, request protocol.ConfigRequest) (int, error) {
	if err := request.Validate(); err != nil {
		return 0, err
	}
	if err := s.requireDevice(ctx, id); err != nil {
		return 0, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running || s.ctx.Err() != nil || !s.pub.Available() {
		return 0, apperr.ErrMQTTUnavailable
	}
	version, err := s.configs.UpdateDesired(ctx, id, request, s.now().UTC())
	if err != nil {
		return 0, err
	}
	s.enqueueLocked(id)
	return version, nil
}

// Restore is called only after all uplink subscriptions are ready.
func (s *ConfigManager) Restore(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running || s.ctx.Err() != nil {
		return fmt.Errorf("config manager stopped")
	}
	ids, err := s.configs.ListDeviceIDs(ctx)
	if err != nil {
		return err
	}
	for _, id := range ids {
		s.enqueueLocked(id)
	}
	return nil
}
func (s *ConfigManager) sendLoop() {
	defer s.workers.Done()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-s.wake:
		}
		s.mu.Lock()
		ids := make([]string, 0, len(s.pending))
		for id := range s.pending {
			ids = append(ids, id)
		}
		clear(s.pending)
		s.mu.Unlock()
		sort.Strings(ids)
		for _, id := range ids {
			if s.ctx.Err() != nil {
				return
			}
			if err := s.publishLatest(id); err != nil {
				slog.Warn("配置下发未确认，等待重连或重启恢复", "device_id", id, "err", err)
			}
		}
	}
}
func (s *ConfigManager) publishLatest(id string) error {
	ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
	defer cancel()
	row, err := s.configs.Snapshot(ctx, id)
	if err != nil {
		return err
	}
	var doc model.RulesConfig
	if err = json.Unmarshal([]byte(row.Revision.ConfigJSON), &doc); err != nil {
		return err
	}
	message := protocol.ConfigMessage{SchemaVersion: 1, DeviceID: id, ConfigVersion: row.Revision.ConfigVersion, IssuedAt: row.Revision.IssuedAt, Rules: doc.Rules}
	raw, err := json.Marshal(message)
	if err != nil {
		return err
	}
	return s.pub.Publish(ctx, "car/"+id+"/config", 1, true, raw)
}
func (s *ConfigManager) IngestAck(ctx context.Context, topic string, raw []byte) error {
	ack, err := protocol.ParseConfigAck(topic, raw)
	if err != nil {
		return err
	}
	if err = s.requireDevice(ctx, ack.DeviceID); err != nil {
		return err
	}
	_, err = s.configs.ApplyAck(ctx, *ack, s.now().UTC())
	return err
}
