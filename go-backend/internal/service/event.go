package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"piguard/go-backend/internal/apperr"
	"piguard/go-backend/internal/model"
	"piguard/go-backend/internal/protocol"
	"piguard/go-backend/internal/realtime"
	"piguard/go-backend/internal/repo"
)

// EventService 接收告警消息并提供查询。
type EventService interface {
	Ingest(ctx context.Context, topic string, payload []byte) error
	List(ctx context.Context, deviceID string, query repo.EventQuery) ([]protocol.EventItem, error)
}

type eventService struct {
	events  repo.EventRepository
	devices repo.DeviceRepository
	hub     *realtime.Hub
	now     func() time.Time
}

func NewEventService(events repo.EventRepository, devices repo.DeviceRepository, hub *realtime.Hub) EventService {
	return &eventService{events: events, devices: devices, hub: hub, now: time.Now}
}

func (s *eventService) Ingest(ctx context.Context, topic string, payload []byte) error {
	event, err := protocol.ParseEvent(topic, payload)
	if err != nil {
		return err
	}
	if _, err := s.devices.Find(ctx, event.DeviceID); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return fmt.Errorf("unknown device %s", event.DeviceID)
		}
		return err
	}
	row := &model.Event{
		EventID: event.EventID, DeviceID: event.DeviceID, Type: event.Type, Action: event.Action,
		Level: event.Level, Payload: string(event.Data), Timestamp: event.Timestamp.UTC(),
	}
	if event.SnapshotID != nil {
		row.SnapshotID = *event.SnapshotID
	}
	if err := s.events.Insert(ctx, row); err != nil {
		if !repo.DuplicateEvent(err) {
			return err
		}
		return s.existing(ctx, event)
	}
	if s.hub != nil {
		s.hub.Publish("event", pushFrom(row), s.now().UTC())
	}
	return nil
}

func (s *eventService) existing(ctx context.Context, event *protocol.Event) error {
	row, err := s.events.Find(ctx, event.EventID)
	if err != nil {
		return err
	}
	if event.SameRecord(row.DeviceID, row.Type, row.Action, row.Level, row.Timestamp, row.Payload, row.SnapshotID) {
		slog.Info("重复告警已忽略", "event_id", event.EventID)
		return nil
	}
	slog.Warn("告警 event_id 冲突，保留首条", "event_id", event.EventID, "device_id", event.DeviceID)
	return nil
}

func (s *eventService) List(ctx context.Context, deviceID string, query repo.EventQuery) ([]protocol.EventItem, error) {
	if _, err := s.devices.Find(ctx, deviceID); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return nil, apperr.ErrDeviceNotFound
		}
		return nil, err
	}
	if query.Type != nil && !protocol.ValidEventType(*query.Type) {
		return nil, &apperr.InvalidParams{Field: "type"}
	}
	if query.Level != nil && !protocol.ValidEventLevel(*query.Level) {
		return nil, &apperr.InvalidParams{Field: "level"}
	}
	if query.Limit == 0 {
		query.Limit = 100
	}
	if query.Limit < 1 || query.Limit > 1000 {
		return nil, &apperr.InvalidParams{Field: "limit"}
	}
	if query.Start != nil && query.End != nil && query.Start.After(*query.End) {
		return nil, &apperr.InvalidParams{Field: "start"}
	}
	rows, err := s.events.List(ctx, deviceID, query)
	if err != nil {
		return nil, err
	}
	items := make([]protocol.EventItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, itemFrom(row))
	}
	return items, nil
}

func itemFrom(row model.Event) protocol.EventItem {
	return protocol.EventItem{
		EventID: row.EventID, Type: row.Type, Action: row.Action, Level: row.Level,
		Timestamp: row.Timestamp.UTC(), Data: []byte(row.Payload), SnapshotID: snapshotPtr(row.SnapshotID),
	}
}

func pushFrom(row *model.Event) protocol.EventPush {
	item := itemFrom(*row)
	return protocol.EventPush{
		EventID: item.EventID, DeviceID: row.DeviceID, EventType: row.Type, Action: item.Action,
		Level: item.Level, Timestamp: item.Timestamp, Data: item.Data, SnapshotID: item.SnapshotID,
	}
}

func snapshotPtr(id string) *string {
	if id == "" {
		return nil
	}
	return &id
}
