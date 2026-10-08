package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"strings"
	"sync"
	"time"

	"piguard/go-backend/internal/apperr"
	"piguard/go-backend/internal/model"
	"piguard/go-backend/internal/protocol"
	"piguard/go-backend/internal/realtime"
	"piguard/go-backend/internal/repo"
)

// Publisher 是服务层看到的 MQTT 发布端口。
// 具体客户端由 mqtt 包实现，服务层不依赖 Paho。
type Publisher interface {
	Available() bool
	Publish(ctx context.Context, topic string, qos byte, retained bool, payload []byte) error
}

// CommandService 负责把网页上的控制请求变成命令记录，并在之后通过 MQTT 下发。
type CommandService interface {
	Create(context.Context, string, protocol.CommandRequest) (string, error)
	Get(context.Context, string) (*protocol.CommandView, error)
}

type CommandManager struct {
	mu       sync.Mutex
	commands repo.CommandRepository
	devices  repo.DeviceRepository
	pub      Publisher
	online   func(string) bool
	hub      *realtime.Hub
	now      func() time.Time
	queue    chan protocol.Command
	ctx      context.Context
	cancel   context.CancelFunc
	running  bool
	workers  sync.WaitGroup
}

func NewCommandService(commands repo.CommandRepository, devices repo.DeviceRepository, pub Publisher, online func(string) bool, hub *realtime.Hub) *CommandManager {
	return &CommandManager{commands: commands, devices: devices, pub: pub, online: online, hub: hub, now: time.Now, queue: make(chan protocol.Command, 256)}
}

func (s *CommandManager) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ctx != nil {
		return fmt.Errorf("command manager already started")
	}
	if err := s.expireLocked(ctx); err != nil {
		return err
	}
	s.ctx, s.cancel = context.WithCancel(ctx)
	s.running = true
	s.workers.Add(2)
	go s.sendLoop()
	go s.expiryLoop()
	return nil
}

func (s *CommandManager) Close() {
	s.mu.Lock()
	s.running = false
	if s.cancel != nil {
		s.cancel()
	}
	s.mu.Unlock()
	s.workers.Wait()
}

func (s *CommandManager) Create(ctx context.Context, id string, request protocol.CommandRequest) (string, error) {
	if err := request.Validate(); err != nil {
		return "", &apperr.InvalidParams{Field: "params"}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.devices.Find(ctx, id); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return "", apperr.ErrDeviceNotFound
		}
		return "", err
	}
	if !s.online(id) {
		return "", apperr.ErrDeviceOffline
	}
	if !s.running || s.ctx.Err() != nil || !s.pub.Available() || len(s.queue) == cap(s.queue) {
		return "", apperr.ErrMQTTUnavailable
	}
	issued := s.now().UTC()
	cmd := protocol.Command{SchemaVersion: 1, CommandID: "cmd-" + rand.Text(), DeviceID: id, Type: request.Type, Params: append(json.RawMessage(nil), request.Params...), IssuedAt: issued, ExpiresAt: issued.Add(10 * time.Second)}
	row := &model.Command{CommandID: cmd.CommandID, DeviceID: id, Type: cmd.Type, Params: string(cmd.Params), Status: "pending", IssuedAt: issued, ExpiresAt: cmd.ExpiresAt}
	if err := s.commands.Insert(ctx, row); err != nil {
		return "", err
	}
	s.broadcast(row)
	s.queue <- cmd // The producer lock reserves the queue slot before insertion.
	return cmd.CommandID, nil
}

func commandView(row *model.Command) *protocol.CommandView {
	return &protocol.CommandView{
		CommandID: row.CommandID, DeviceID: row.DeviceID, Type: row.Type,
		Params: commandJSON(row.Params), Status: row.Status,
		IssuedAt: row.IssuedAt.UTC(), ExpiresAt: row.ExpiresAt.UTC(),
		AckAt: row.AckAt, ExecutedAt: row.ExecutedAt,
		Result: commandJSON(row.Result), Error: commandJSON(row.Error),
	}
}

func commandJSON(raw string) json.RawMessage {
	if raw == "" {
		return nil
	}
	return json.RawMessage(raw)
}
func (s *CommandManager) broadcast(row *model.Command) {
	s.hub.Publish("command_update", commandView(row), s.now().UTC())
}
func (s *CommandManager) transition(ctx context.Context, row *model.Command, from ...string) error {
	changed, err := s.commands.Transition(ctx, row, from)
	if err == nil && changed {
		s.broadcast(row)
	}
	return err
}
func pending(status string) bool { return status == "pending" || status == "sent" }
func (s *CommandManager) timeoutLocked(ctx context.Context, row *model.Command) error {
	if pending(row.Status) && !s.now().Before(row.ExpiresAt) {
		row.Status = "timeout"
		return s.transition(ctx, row, "pending", "sent")
	}
	return nil
}
func (s *CommandManager) Get(ctx context.Context, id string) (*protocol.CommandView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	row, err := s.commands.Find(ctx, id)
	if errors.Is(err, repo.ErrNotFound) {
		return nil, apperr.ErrCommandNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := s.timeoutLocked(ctx, row); err != nil {
		return nil, err
	}
	return commandView(row), nil
}
func (s *CommandManager) IngestAck(ctx context.Context, topic string, payload []byte) error {
	a, err := protocol.ParseCommandAck(topic, payload)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	received := s.now().UTC()
	row, err := s.commands.Find(ctx, a.CommandID)
	if err != nil {
		return fmt.Errorf("Ack lookup: %w", err)
	}
	if row.DeviceID != a.DeviceID {
		return fmt.Errorf("Ack belongs to a different device")
	}
	if !pending(row.Status) {
		reason, level := "conflict", slog.LevelWarn
		if row.Status == "timeout" {
			reason = "late"
		} else if sameCommandAck(row, a) {
			reason, level = "duplicate", slog.LevelInfo
		}
		s.logIgnoredAck(ctx, level, reason, row, a, received)
		return nil
	}
	if !received.Before(row.ExpiresAt) {
		row.Status = "timeout"
		s.logIgnoredAck(ctx, slog.LevelWarn, "late", row, a, received)
		return s.transition(ctx, row, "pending", "sent")
	}
	row.Status, row.AckAt, row.ExecutedAt = a.Status, &received, &a.ExecutedAt
	row.Result, row.Error = string(a.Result), ""
	if a.Error != nil {
		raw, err := json.Marshal(a.Error)
		if err != nil {
			return err
		}
		row.Error = string(raw)
	}
	return s.transition(ctx, row, "pending", "sent")
}

func sameCommandAck(row *model.Command, ack *protocol.CommandAck) bool {
	if row.Status != ack.Status || row.ExecutedAt == nil || !row.ExecutedAt.Equal(ack.ExecutedAt) {
		return false
	}
	if ack.Status == "failed" {
		var stored protocol.CommandError
		return ack.Error != nil && json.Unmarshal([]byte(row.Error), &stored) == nil && stored == *ack.Error
	}
	var stored, received any
	storedDecoder := json.NewDecoder(strings.NewReader(row.Result))
	receivedDecoder := json.NewDecoder(bytes.NewReader(ack.Result))
	storedDecoder.UseNumber()
	receivedDecoder.UseNumber()
	return storedDecoder.Decode(&stored) == nil && receivedDecoder.Decode(&received) == nil && reflect.DeepEqual(stored, received)
}

func (s *CommandManager) logIgnoredAck(ctx context.Context, level slog.Level, reason string, row *model.Command, ack *protocol.CommandAck, received time.Time) {
	slog.Log(ctx, level, "忽略命令回执", "reason", reason, "command_id", row.CommandID, "device_id", row.DeviceID,
		"status", row.Status, "ack_status", ack.Status, "executed_at", ack.ExecutedAt, "received_at", received, "expires_at", row.ExpiresAt)
}

func (s *CommandManager) expireLocked(ctx context.Context) error {
	rows, err := s.commands.Expired(ctx, s.now().UTC())
	if err != nil {
		return err
	}
	for i := range rows {
		rows[i].Status = "timeout"
		if err := s.transition(ctx, &rows[i], "pending", "sent"); err != nil {
			return err
		}
	}
	return nil
}
func (s *CommandManager) expiryLoop() {
	defer s.workers.Done()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.mu.Lock()
			err := s.expireLocked(s.ctx)
			s.mu.Unlock()
			if err != nil && s.ctx.Err() == nil {
				slog.Error("命令超时扫描失败", "err", err)
			}
		}
	}
}
func (s *CommandManager) sendLoop() {
	defer s.workers.Done()
	for {
		select {
		case <-s.ctx.Done():
			return
		case cmd := <-s.queue:
			if s.ctx.Err() != nil {
				return
			}
			s.send(cmd)
		}
	}
}
func (s *CommandManager) send(cmd protocol.Command) {
	s.mu.Lock()
	row, err := s.commands.Find(s.ctx, cmd.CommandID)
	if err == nil {
		err = s.timeoutLocked(s.ctx, row)
	}
	s.mu.Unlock()
	if err != nil {
		slog.Error("读取待发送命令失败", "command_id", cmd.CommandID, "err", err)
		return
	}
	if row.Status != "pending" {
		return
	}
	raw, err := json.Marshal(cmd)
	if err != nil {
		slog.Error("命令编码失败", "err", err)
		return
	}
	ctx, cancel := context.WithDeadline(s.ctx, cmd.ExpiresAt)
	err = s.pub.Publish(ctx, "car/"+cmd.DeviceID+"/commands", 1, false, raw)
	cancel()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ctx.Err() != nil {
		return
	}
	row, lookupErr := s.commands.Find(s.ctx, cmd.CommandID)
	if lookupErr != nil {
		slog.Error("读取发送结果失败", "err", lookupErr)
		return
	}
	if expireErr := s.timeoutLocked(s.ctx, row); expireErr != nil {
		slog.Error("命令过期更新失败", "err", expireErr)
		return
	}
	if row.Status != "pending" {
		return
	}
	switch {
	case err == nil:
		row.Status = "sent"
	case errors.Is(err, apperr.ErrMQTTUnavailable):
		row.Status = "failed"
		failure, _ := json.Marshal(protocol.CommandError{Code: "MQTT_UNAVAILABLE", Message: "command could not be published"})
		row.Error = string(failure)
	default:
		slog.Warn("命令发送结果不确定，等待回执或到期", "command_id", cmd.CommandID, "err", err)
		return
	}
	if err := s.transition(s.ctx, row, "pending"); err != nil {
		slog.Error("命令发送状态更新失败", "err", err)
	}
}
