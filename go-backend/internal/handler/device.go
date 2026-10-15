package handler

import (
	"encoding/json"
	"piguard/go-backend/internal/model"
	"piguard/go-backend/internal/service"
	"time"

	"github.com/gin-gonic/gin"
)

// DeviceHandler 处理设备列表、设备详情和最新状态。
type DeviceHandler struct {
	svc service.DeviceService
}

func NewDeviceHandler(svc service.DeviceService) *DeviceHandler {
	return &DeviceHandler{svc: svc}
}

// List 对应 GET /api/v1/devices。
func (h *DeviceHandler) List(c *gin.Context) {
	items, err := h.svc.List(c.Request.Context())
	if err != nil {
		writeError(c, err)
		return
	}
	views := make([]deviceView, 0, len(items))
	for _, d := range items {
		views = append(views, toDeviceView(d, false))
	}
	writeOK(c, views)
}

// Get 对应 GET /api/v1/devices/:device_id。
func (h *DeviceHandler) Get(c *gin.Context) {
	d, err := h.svc.Get(c.Request.Context(), c.Param("device_id"))
	if err != nil {
		writeError(c, err)
		return
	}
	writeOK(c, toDeviceView(*d, true))
}

// State 对应 GET /api/v1/devices/:device_id/state。
func (h *DeviceHandler) State(c *gin.Context) {
	state, err := h.svc.State(c.Request.Context(), c.Param("device_id"))
	if err != nil {
		writeError(c, err)
		return
	}
	writeOK(c, state)
}

type deviceView struct {
	DeviceID        string             `json:"device_id"`
	Name            string             `json:"name"`
	Online          bool               `json:"online"`
	LastSeenAt      *time.Time         `json:"last_seen_at"`
	ConfigVersion   int                `json:"config_version"`
	Mode            string             `json:"mode,omitempty"`
	SoftwareVersion string             `json:"software_version,omitempty"`
	Sensors         *map[string]string `json:"sensors,omitempty"`
}

func toDeviceView(d model.Device, detail bool) deviceView {
	v := deviceView{DeviceID: d.DeviceID, Name: d.Name, Online: d.Online, LastSeenAt: d.LastSeenAt, ConfigVersion: d.ReportedConfigVersion}
	if v.LastSeenAt != nil {
		utc := v.LastSeenAt.UTC()
		v.LastSeenAt = &utc
	}
	if detail {
		v.Mode, v.SoftwareVersion = d.Mode, d.SoftwareVersion
		sensors := map[string]string{}
		if d.SensorsJSON != "" {
			_ = json.Unmarshal([]byte(d.SensorsJSON), &sensors)
		}
		v.Sensors = &sensors
	}
	return v
}
