package service

import (
	"context"

	"piguard/go-backend/internal/model"
	"piguard/go-backend/internal/repo"
)

// EventService 提供告警记录查询。MQTT 事件接入还没做。
type EventService interface {
	List(ctx context.Context, deviceID string) ([]model.Event, error)
}

type eventService struct {
	events repo.EventRepository
}

func NewEventService(events repo.EventRepository) EventService {
	return &eventService{events: events}
}

func (s *eventService) List(ctx context.Context, deviceID string) ([]model.Event, error) {
	return s.events.List(ctx, deviceID)
}
