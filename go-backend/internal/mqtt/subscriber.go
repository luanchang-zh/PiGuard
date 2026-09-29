package mqtt

import (
	"fmt"
	"log/slog"
)

// uplinkFilters 是平台稍后要订阅的上行主题。加号匹配任意一台设备。
// 现在不订阅：遥测大约每秒两条，没有入库处理时订了只会空转。
var uplinkFilters = []string{
	"car/+/telemetry",
	"car/+/events",
	"car/+/status",
	"car/+/command-acks",
	"car/+/config-acks",
}

// Subscriber 预留给遥测、告警、状态和回执的回调。
type Subscriber struct {
	client *Client
}

func NewSubscriber(client *Client) *Subscriber {
	return &Subscriber{client: client}
}

// Start 确认客户端已经连上，并记录计划订阅的主题。当前不向 Broker 注册这些主题。
func (s *Subscriber) Start() error {
	if s.client == nil || s.client.Raw() == nil {
		return fmt.Errorf("MQTT 客户端未连接")
	}
	slog.Info("MQTT 已连接，上行订阅尚未注册", "topics", uplinkFilters)
	return nil
}
