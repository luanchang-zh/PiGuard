package handler

import (
	"strconv"
	"time"

	"piguard/go-backend/internal/apperr"
	"piguard/go-backend/internal/repo"
	"piguard/go-backend/internal/service"

	"github.com/gin-gonic/gin"
)

// EventHandler 处理告警记录查询。
type EventHandler struct {
	svc service.EventService
}

func NewEventHandler(svc service.EventService) *EventHandler {
	return &EventHandler{svc: svc}
}

// List 对应 GET /api/v1/devices/:device_id/events。
func (h *EventHandler) List(c *gin.Context) {
	query := repo.EventQuery{Limit: 100}
	params := c.Request.URL.Query()
	if params.Has("type") {
		value := params.Get("type")
		query.Type = &value
	}
	if params.Has("level") {
		value := params.Get("level")
		query.Level = &value
	}
	for name, dst := range map[string]**time.Time{"start": &query.Start, "end": &query.End} {
		if params.Has(name) {
			at, err := time.Parse(time.RFC3339Nano, params.Get(name))
			if err != nil {
				writeError(c, &apperr.InvalidParams{Field: name})
				return
			}
			utc := at.UTC()
			*dst = &utc
		}
	}
	if params.Has("limit") {
		limit, err := strconv.Atoi(params.Get("limit"))
		if err != nil || limit < 1 || limit > 1000 {
			writeError(c, &apperr.InvalidParams{Field: "limit"})
			return
		}
		query.Limit = limit
	}
	items, err := h.svc.List(c.Request.Context(), c.Param("device_id"), query)
	if err != nil {
		writeError(c, err)
		return
	}
	writeOK(c, gin.H{"items": items})
}
