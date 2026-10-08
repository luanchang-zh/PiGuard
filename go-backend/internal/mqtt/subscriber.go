package mqtt

import (
	"context"
	"fmt"
	paho "github.com/eclipse/paho.mqtt.golang"
	"log/slog"
	"piguard/go-backend/internal/service"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var uplinkFilters = map[string]byte{"car/+/telemetry": 0, "car/+/status": 1, "car/+/command-acks": 1}

// Subscriber is configured before Connect; Start verifies the first subscription.
type Subscriber struct {
	client   *Client
	monitor  *service.Monitor
	commands *service.CommandManager
	ctx      context.Context
	cancel   context.CancelFunc
	started  atomic.Bool
	mu       sync.Mutex
}

func NewSubscriber(client *Client, monitor *service.Monitor, commands *service.CommandManager) *Subscriber {
	ctx, cancel := context.WithCancel(context.Background())
	s := &Subscriber{client: client, monitor: monitor, commands: commands, ctx: ctx, cancel: cancel}
	client.opts.SetOnConnectHandler(func(raw paho.Client) {
		if !s.started.Load() {
			return
		}
		go func() {
			for s.ctx.Err() == nil && raw.IsConnectionOpen() {
				if err := s.subscribe(raw); err == nil {
					return
				} else {
					slog.Warn("恢复 MQTT 订阅失败", "err", err)
				}
				select {
				case <-s.ctx.Done():
					return
				case <-time.After(time.Second):
				}
			}
		}()
	})
	client.opts.SetConnectionLostHandler(func(_ paho.Client, err error) {
		slog.Warn("MQTT 连接已断开，等待重连", "err", err)
		ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
		defer cancel()
		if err := monitor.Disconnected(ctx); err != nil {
			slog.Error("标记设备离线失败", "err", err)
		}
	})
	return s
}

func (s *Subscriber) Start() error {
	if s.client == nil || s.client.Raw() == nil {
		return fmt.Errorf("MQTT 客户端未连接")
	}
	s.started.Store(true)
	if err := s.subscribe(s.client.Raw()); err != nil {
		s.started.Store(false)
		return err
	}
	return nil
}

func (s *Subscriber) subscribe(raw paho.Client) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ctx.Err() != nil {
		return s.ctx.Err()
	}
	token := raw.SubscribeMultiple(uplinkFilters, func(_ paho.Client, msg paho.Message) {
		ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
		defer cancel()
		if strings.HasSuffix(msg.Topic(), "/command-acks") {
			if err := s.commands.IngestAck(ctx, msg.Topic(), msg.Payload()); err != nil {
				slog.Warn("拒绝命令回执", "topic", msg.Topic(), "err", err)
			}
			return
		}
		s.monitor.LogIngest(ctx, msg.Topic(), msg.Payload())
	})
	if !token.WaitTimeout(s.client.timeout) {
		return fmt.Errorf("MQTT 订阅超时")
	}
	if err := token.Error(); err != nil {
		return fmt.Errorf("MQTT 订阅: %w", err)
	}
	slog.Info("MQTT 上行订阅已就绪", "topics", uplinkFilters)
	return nil
}

func (s *Subscriber) Close() { s.started.Store(false); s.cancel() }
