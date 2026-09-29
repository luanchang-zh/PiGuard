package mqtt

import (
	"context"
	"fmt"

	"piguard/go-backend/internal/apperr"
)

// Publisher 负责把命令和配置发到 Broker。
// 下行主题是 car/{device_id}/commands 和 car/{device_id}/config，发送逻辑尚未实现。
type Publisher struct {
	client *Client
}

func NewPublisher(client *Client) *Publisher {
	return &Publisher{client: client}
}

// Publish 在命令和配置接口接通后才会真正发送。
// QoS 和 retained 由调用方按规范传入：命令 QoS 1 不保留，配置 QoS 1 且保留。
func (p *Publisher) Publish(context.Context, string, byte, bool, []byte) error {
	if p.client == nil || p.client.Raw() == nil {
		return fmt.Errorf("MQTT 客户端未连接")
	}
	return apperr.ErrNotImplemented
}
