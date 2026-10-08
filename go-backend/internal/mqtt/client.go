// Package mqtt 维持和 Mosquitto 的连接。
// 启动时必须连上，否则进程退出；Subscriber 负责订阅与重连恢复。
package mqtt

import (
	"errors"
	"fmt"
	"log/slog"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"
)

// 初次连接最多试 5 次，每次间隔 1 秒。
// Compose 里虽然会等 Broker 健康，进程内仍留这点余量，躲开健康检查和端口可连之间的空隙。
const connectAttempts = 5

// Options 是连接 Broker 所需的参数，由启动流程从配置转换而来。
type Options struct {
	Broker         string
	ClientID       string
	Username       string
	Password       string
	ConnectTimeout time.Duration
}

// Client 是已配置好的 MQTT 客户端。Connect 成功之后才可以发布或订阅。
type Client struct {
	broker  string
	timeout time.Duration
	opts    *paho.ClientOptions
	raw     paho.Client
}

// NewClient 组装 Paho 选项。自动重连只处理“曾经连上之后又断开”，挡不住第一次就失败。
func NewClient(opts Options) *Client {
	pahoOpts := paho.NewClientOptions().
		AddBroker(opts.Broker).
		SetClientID(opts.ClientID).
		SetAutoReconnect(true).
		SetConnectRetry(false).
		SetOrderMatters(true).
		SetKeepAlive(30 * time.Second).
		SetConnectionLostHandler(func(_ paho.Client, err error) {
			slog.Warn("MQTT 连接已断开，客户端将尝试重连", "err", err)
		})

	if opts.Username != "" {
		pahoOpts.SetUsername(opts.Username)
		pahoOpts.SetPassword(opts.Password)
	}

	timeout := opts.ConnectTimeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	pahoOpts.SetConnectTimeout(timeout)
	return &Client{broker: opts.Broker, timeout: timeout, opts: pahoOpts}
}

// Connect 连接 Broker。多次失败后返回错误，调用方应结束进程。
func (c *Client) Connect() error {
	var last error
	for attempt := 1; attempt <= connectAttempts; attempt++ {
		raw := paho.NewClient(c.opts)
		token := raw.Connect()
		if token.WaitTimeout(c.timeout) && token.Error() == nil {
			c.raw = raw
			slog.Info("已连接 MQTT Broker", "broker", c.broker)
			return nil
		}

		if err := token.Error(); err != nil {
			last = err
		} else {
			last = errors.New("连接超时")
		}
		raw.Disconnect(200)
		slog.Warn("连接 MQTT Broker 失败", "broker", c.broker, "attempt", attempt, "err", last)
		if attempt < connectAttempts {
			time.Sleep(time.Second)
		}
	}
	return fmt.Errorf("无法连接 MQTT Broker %s: %w", c.broker, last)
}

// Raw 把底层客户端交给发布器。未连接时返回 nil。
func (c *Client) Raw() paho.Client {
	return c.raw
}

// Disconnect 在进程退出时通知 Broker 本端离开。
func (c *Client) Disconnect() {
	if c.raw != nil && c.raw.IsConnected() {
		c.raw.Disconnect(1000)
	}
}
