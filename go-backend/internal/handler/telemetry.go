package handler

import (
	"piguard/go-backend/internal/apperr"
	"piguard/go-backend/internal/repo"
	"piguard/go-backend/internal/service"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// TelemetryHandler 处理历史遥测查询。
type TelemetryHandler struct {
	svc service.TelemetryService
}

func NewTelemetryHandler(svc service.TelemetryService) *TelemetryHandler {
	return &TelemetryHandler{svc: svc}
}

// List 对应 GET /api/v1/devices/:device_id/telemetry。
func (h *TelemetryHandler) List(c *gin.Context) {
	query := repo.TelemetryQuery{Limit: 100}
	params := c.Request.URL.Query()
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
