package mqtt

import (
	"context"
	"fmt"
	"time"

	"piguard/go-backend/internal/apperr"
)

// Publisher 负责把命令和配置发到 Broker。
// 发布后不能确定交付结果的错误单独返回，不能声称设备没有执行。
type Publisher struct {
	client *Client
}

func NewPublisher(client *Client) *Publisher {
	return &Publisher{client: client}
}

func (p *Publisher) Available() bool {
	return p.client != nil && p.client.Raw() != nil && p.client.Raw().IsConnectionOpen()
}
func (p *Publisher) Publish(ctx context.Context, topic string, qos byte, retained bool, payload []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !p.Available() {
		return apperr.ErrMQTTUnavailable
	}
	token := p.client.Raw().Publish(topic, qos, retained, payload)
	waitCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	select {
	case <-token.Done():
		if err := token.Error(); err != nil {
			return fmt.Errorf("%w: %v", apperr.ErrPublishUncertain, err)
		}
		return nil
	case <-waitCtx.Done():
		return fmt.Errorf("%w: %v", apperr.ErrPublishUncertain, waitCtx.Err())
	}
}
