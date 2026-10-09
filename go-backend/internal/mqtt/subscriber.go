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

var uplinkFilters = map[string]byte{"car/+/telemetry": 0, "car/+/status": 1, "car/+/command-acks": 1, "car/+/events": 1, "car/+/config-acks": 1}

type ConfigSync interface {
	IngestAck(context.Context, string, []byte) error
	Restore(context.Context) error
}

// Subscriber is configured before Connect; Start verifies the first subscription.
type Subscriber struct {
	client   *Client
	monitor  *service.Monitor
	commands *service.CommandManager
	events   service.EventService
	configs  ConfigSync
	ctx      context.Context
	cancel   context.CancelFunc
	started  atomic.Bool
	mu       sync.Mutex
}

func NewSubscriber(client *Client, monitor *service.Monitor, commands *service.CommandManager, events service.EventService, configs ...ConfigSync) *Subscriber {
	ctx, cancel := context.WithCancel(context.Background())
	s := &Subscriber{client: client, monitor: monitor, commands: commands, events: events, ctx: ctx, cancel: cancel}
	if len(configs) > 0 {
		s.configs = configs[0]
	}
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
		if monitor == nil {
			return
		}
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
		if s.ctx.Err() != nil {
			return
		}
		ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
		defer cancel()
		switch {
		case strings.HasSuffix(msg.Topic(), "/config-acks"):
			if s.configs == nil {
				slog.Warn("配置服务未配置")
				return
			}
			if err := s.configs.IngestAck(ctx, msg.Topic(), msg.Payload()); err != nil {
				slog.Warn("拒绝配置回执", "topic", msg.Topic(), "err", err)
			}
		case strings.HasSuffix(msg.Topic(), "/command-acks"):
			if err := s.commands.IngestAck(ctx, msg.Topic(), msg.Payload()); err != nil {
				slog.Warn("拒绝命令回执", "topic", msg.Topic(), "err", err)
			}
		case strings.HasSuffix(msg.Topic(), "/events"):
			if s.events == nil {
				slog.Warn("告警服务未配置", "topic", msg.Topic())
				return
			}
			if err := s.events.Ingest(ctx, msg.Topic(), msg.Payload()); err != nil {
				slog.Warn("拒绝告警事件", "topic", msg.Topic(), "err", err)
			}
		default:
			s.monitor.LogIngest(ctx, msg.Topic(), msg.Payload())
		}
	})
	timer := time.NewTimer(s.client.timeout)
	defer timer.Stop()
	select {
	case <-s.ctx.Done():
		return s.ctx.Err()
	case <-timer.C:
		return fmt.Errorf("MQTT 订阅超时")
	case <-token.Done():
	}
	if err := token.Error(); err != nil {
		return fmt.Errorf("MQTT 订阅: %w", err)
	}
	// Paho completes a SUBACK token without an error even when the Broker
	// returns 0x80 (rejected). Verify every granted QoS before opening sends.
	if result, ok := token.(interface{ Result() map[string]byte }); ok {
		granted := result.Result()
		for topic, qos := range uplinkFilters {
			if actual, exists := granted[topic]; !exists || actual != qos {
				return fmt.Errorf("MQTT 订阅 %s 未获得 QoS %d", topic, qos)
			}
		}
	}
	slog.Info("MQTT 上行订阅已就绪", "topics", uplinkFilters)
	if s.ctx.Err() != nil || !s.client.subscriptionsReady(raw) {
		return fmt.Errorf("MQTT 订阅属于已结束的会话")
	}
	if s.configs != nil {
		ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
		defer cancel()
		if err := s.configs.Restore(ctx); err != nil {
			return fmt.Errorf("恢复最新配置: %w", err)
		}
	}
	return nil
}

func (s *Subscriber) Close() {
	s.started.Store(false)
	s.cancel()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.client.suspendPublishing(s.client.Raw())
}
