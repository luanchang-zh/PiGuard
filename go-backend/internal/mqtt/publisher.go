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
	return p.client != nil && p.client.publishClient() != nil
}
func (p *Publisher) Publish(ctx context.Context, topic string, qos byte, retained bool, payload []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if p.client == nil {
		return apperr.ErrMQTTUnavailable
	}
	raw := p.client.publishClient()
	if raw == nil {
		return apperr.ErrMQTTUnavailable
	}
	waitCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	// Paho Publish itself may wait on its outbound queue. Cancellation closes
	// that session before returning, so it cannot send an old packet after a
	// new session restores a newer retained value.
	result := make(chan error, 1)
	go func() {
		token := raw.Publish(topic, qos, retained, payload)
		select {
		case <-token.Done():
			result <- token.Error()
		case <-waitCtx.Done():
		}
	}()
	select {
	case err := <-result:
		if err != nil {
			p.client.abortPublish(raw, err)
			return fmt.Errorf("%w: %v", apperr.ErrPublishUncertain, err)
		}
		return nil
	case <-waitCtx.Done():
		p.client.abortPublish(raw, waitCtx.Err())
		return fmt.Errorf("%w: %v", apperr.ErrPublishUncertain, waitCtx.Err())
	}
}
