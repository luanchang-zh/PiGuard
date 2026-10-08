package service

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"piguard/go-backend/internal/apperr"
	"piguard/go-backend/internal/protocol"
	"piguard/go-backend/internal/repo"
)

type HistoryItem struct {
	Timestamp   time.Time                      `json:"timestamp"`
	Speed       *float64                       `json:"speed_kmh"`
	Distance    *float64                       `json:"distance_m"`
	Temperature *float64                       `json:"temperature_c"`
	YawRate     *float64                       `json:"yaw_rate_deg_s"`
	LaneOffset  *float64                       `json:"lane_offset_ratio"`
	RiskLevel   string                         `json:"risk_level"`
	Samples     map[string]protocol.SampleInfo `json:"samples"`
	Freshness   map[string]string              `json:"freshness"`
}

// TelemetryService exposes filtered snapshots and their sampling metadata.
type TelemetryService interface {
	List(ctx context.Context, deviceID string, query repo.TelemetryQuery) ([]HistoryItem, error)
}

type telemetryService struct {
	telemetry repo.TelemetryRepository
	devices   repo.DeviceRepository
}

func NewTelemetryService(telemetry repo.TelemetryRepository, devices repo.DeviceRepository) TelemetryService {
	return &telemetryService{telemetry: telemetry, devices: devices}
}

func (s *telemetryService) List(ctx context.Context, deviceID string, query repo.TelemetryQuery) ([]HistoryItem, error) {
	if _, err := s.devices.Find(ctx, deviceID); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return nil, apperr.ErrDeviceNotFound
		}
		return nil, err
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
	rows, err := s.telemetry.List(ctx, deviceID, query)
	if err != nil {
		return nil, err
	}
	items := make([]HistoryItem, 0, len(rows))
	for _, r := range rows {
		item := HistoryItem{Timestamp: r.Timestamp.UTC(), Speed: r.Speed, Distance: r.Distance,
			Temperature: r.Temperature, YawRate: r.YawRate, LaneOffset: r.LaneOffset, RiskLevel: r.RiskLevel,
			Samples: map[string]protocol.SampleInfo{}, Freshness: map[string]string{}}
		if r.PayloadJSON != "" {
			var t protocol.Telemetry
			if err := json.Unmarshal([]byte(r.PayloadJSON), &t); err != nil {
				return nil, err
			}
			at := r.ReceivedAt
			if at.IsZero() {
				at = r.Timestamp
			}
			view := protocol.View(deviceID, false, &t, at)
			item.Samples, item.Freshness = view.Samples, view.Freshness
		}
		items = append(items, item)
	}
	return items, nil
}
