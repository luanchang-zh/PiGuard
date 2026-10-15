package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
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
	FrameSaved(context.Context, string, string, string) error
	Confirm(context.Context, string, []json.RawMessage) ([]string, error)
}

type eventService struct {
	events  repo.EventRepository
	devices repo.DeviceRepository
	hub     *realtime.Hub
	frames  FrameService
	now     func() time.Time
	// Serialize creation and image updates so a pending creation cannot follow its update.
	publication sync.Mutex
}

func NewEventService(events repo.EventRepository, devices repo.DeviceRepository, hub *realtime.Hub, frames FrameService) EventService {
	return &eventService{events: events, devices: devices, hub: hub, frames: frames, now: time.Now}
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
	if event.FrameID != nil {
		row.FrameID = *event.FrameID
	}
	s.publication.Lock()
	defer s.publication.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.events.Insert(ctx, row); err != nil {
		if !repo.DuplicateEvent(err) {
			return err
		}
		return s.existing(ctx, event)
	}
	if s.hub != nil {
		image := s.notificationImage(ctx, *row)
		s.hub.Publish("event", pushFrom(row, image), s.now().UTC())
	}
	return nil
}

func (s *eventService) existing(ctx context.Context, event *protocol.Event) error {
	row, err := s.events.Find(ctx, event.EventID)
	if err != nil {
		return err
	}
	if event.SameRecord(row.DeviceID, row.Type, row.Action, row.Level, row.Timestamp, row.Payload, row.SnapshotID, row.FrameID) {
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
		image, err := s.image(ctx, row)
		if err != nil {
			return nil, err
		}
		items = append(items, itemFrom(row, image))
	}
	return items, nil
}

func itemFrom(row model.Event, image protocol.EventImage) protocol.EventItem {
	return protocol.EventItem{
		EventID: row.EventID, Type: row.Type, Action: row.Action, Level: row.Level,
		Timestamp: row.Timestamp.UTC(), Data: []byte(row.Payload), EventImage: image,
	}
}

func pushFrom(row *model.Event, image protocol.EventImage) protocol.EventPush {
	item := itemFrom(*row, image)
	return protocol.EventPush{
		EventID: item.EventID, DeviceID: row.DeviceID, EventType: row.Type, Action: item.Action,
		Level: item.Level, Timestamp: item.Timestamp, Data: item.Data, EventImage: image,
	}
}

func (s *eventService) image(ctx context.Context, row model.Event) (protocol.EventImage, error) {
	if s.frames != nil {
		return s.frames.ResolveEventImage(ctx, row.DeviceID, row.SnapshotID, row.FrameID)
	}
	view := protocol.EventImage{FrameID: snapshotPtr(row.FrameID), SnapshotState: "none"}
	if row.SnapshotID != "" || row.FrameID != "" {
		view.SnapshotState = "unavailable"
		return view, fmt.Errorf("event image resolver is not configured")
	}
	return view, nil
}

func (s *eventService) notificationImage(ctx context.Context, row model.Event) protocol.EventImage {
	view, err := s.image(ctx, row)
	if err != nil {
		slog.Error("已保存告警的图片解析失败", "event_id", row.EventID, "err", err)
		return protocol.EventImage{FrameID: snapshotPtr(row.FrameID), SnapshotState: "unavailable"}
	}
	return view
}

func (s *eventService) FrameSaved(ctx context.Context, device, snapshotID, frameID string) error {
	if s.hub == nil {
		return nil
	}
	const batchSize = 64
	var cursor uint
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		// Release between bounded batches so event ingestion can continue.
		s.publication.Lock()
		rows, err := s.events.ImageReferences(ctx, device, snapshotID, frameID, cursor, batchSize)
		if err == nil {
			for _, row := range rows {
				view := s.notificationImage(ctx, row)
				s.hub.Publish("event_image_update", protocol.EventImageUpdate{EventID: row.EventID, DeviceID: row.DeviceID, EventImage: view}, s.now().UTC())
				cursor = row.ID
			}
		}
		s.publication.Unlock()
		if err != nil {
			return err
		}
		if len(rows) < batchSize {
			return nil
		}
	}
}

func snapshotPtr(id string) *string {
	if id == "" {
		return nil
	}
	return &id
}

// Confirm only acknowledges exact immutable records owned by this device.
func (s *eventService) Confirm(ctx context.Context, device string, payloads []json.RawMessage) ([]string, error) {
	if len(payloads) == 0 || len(payloads) > 64 {
		return nil, &apperr.InvalidParams{Field: "events"}
	}
	parsed := make([]*protocol.Event, 0, len(payloads))
	for _, raw := range payloads {
		event, err := protocol.ParseEvent("car/"+device+"/events", raw)
		if err != nil {
			return nil, &apperr.InvalidParams{Field: "events"}
		}
		parsed = append(parsed, event)
	}
	confirmed := []string{}
	for _, event := range parsed {
		row, err := s.events.Find(ctx, event.EventID)
		if errors.Is(err, repo.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if event.SameRecord(row.DeviceID, row.Type, row.Action, row.Level, row.Timestamp, row.Payload, row.SnapshotID, row.FrameID) {
			confirmed = append(confirmed, event.EventID)
		}
	}
	return confirmed, nil
}
