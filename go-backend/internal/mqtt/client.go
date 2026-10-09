// Package mqtt maintains a connection to the Broker. Reconnection uses a fresh
// clean session; only the application restores persisted, latest configuration.
package mqtt

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"sync"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"
	"golang.org/x/net/proxy"
)

const connectAttempts = 5
const mqttWriteTimeout = time.Second

type Options struct {
	Broker         string
	ClientID       string
	Username       string
	Password       string
	ConnectTimeout time.Duration
}

type Client struct {
	broker  string
	timeout time.Duration
	opts    *paho.ClientOptions
	mu      sync.RWMutex
	raw     paho.Client
	current *connection
	ready   bool
	closed  bool
	ctx     context.Context
	cancel  context.CancelFunc
	lost    chan *connection
	workers sync.WaitGroup
}

// Paho retains unacknowledged QoS1 messages even with CleanSession=true, and
// auto-reconnect replays them concurrently with OnConnect. Reusing that queue
// could overwrite a newer retained config or replay an old action command.
func NewClient(opts Options) *Client {
	timeout := opts.ConnectTimeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	pahoOpts := paho.NewClientOptions().AddBroker(opts.Broker).
		SetClientID(opts.ClientID).SetAutoReconnect(false).SetConnectRetry(false).
		SetCleanSession(true).SetResumeSubs(false).SetProtocolVersion(4).
		SetOrderMatters(true).SetKeepAlive(30 * time.Second).
		SetConnectTimeout(timeout).SetWriteTimeout(mqttWriteTimeout)
	if opts.Username != "" {
		pahoOpts.SetUsername(opts.Username).SetPassword(opts.Password)
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Client{broker: opts.Broker, timeout: timeout, opts: pahoOpts,
		ctx: ctx, cancel: cancel, lost: make(chan *connection, 1)}
}

// Connect must succeed initially. Subsequent losses are handled by one worker
// that closes the old transport before creating a new Paho client and store.
func (c *Client) Connect() error {
	var last error
	for attempt := 1; attempt <= connectAttempts; attempt++ {
		if last = c.connectOnce(false); last == nil {
			c.workers.Add(1)
			go c.reconnectLoop()
			return nil
		}
		slog.Warn("连接 MQTT Broker 失败", "broker", c.broker, "attempt", attempt, "err", last)
		if attempt < connectAttempts && !c.waitRetry() {
			break
		}
	}
	return fmt.Errorf("无法连接 MQTT Broker %s: %w", c.broker, last)
}

func (c *Client) connectOnce(notify bool) error {
	sessionCtx, cancel := context.WithCancel(c.ctx)
	s := &connection{ctx: sessionCtx, cancel: cancel}
	opts := *c.opts
	opts.SetStore(paho.NewMemoryStore())
	opts.SetOnConnectHandler(nil) // Notify only after assigning the new session.
	opts.SetConnectionLostHandler(func(_ paho.Client, err error) { c.connectionLost(s, err) })
	opts.SetCustomOpenConnectionFn(func(uri *url.URL, options paho.ClientOptions) (net.Conn, error) {
		var conn net.Conn
		var err error
		if c.opts.CustomOpenConnectionFn != nil {
			conn, err = c.opts.CustomOpenConnectionFn(uri, options)
		} else {
			conn, err = dialMQTT(sessionCtx, uri, options)
		}
		if err != nil {
			return nil, err
		}
		// Bound priority writes (SUBSCRIBE/ACK/DISCONNECT) too: Paho only
		// applies WriteTimeout to its normal PUBLISH path.
		conn = &boundedConnection{Conn: conn}
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.ctx.Err() != nil {
			conn.Close()
			return nil, s.ctx.Err()
		}
		s.transport = conn
		return conn, nil
	})
	s.raw = paho.NewClient(&opts)
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		s.close()
		return context.Canceled
	}
	previous := c.current
	c.current, c.raw, c.ready = s, s.raw, false
	c.mu.Unlock()
	if previous != nil {
		previous.close()
	}
	token := s.raw.Connect()
	ctx, stop := context.WithTimeout(sessionCtx, c.timeout)
	defer stop()
	select {
	case <-token.Done():
		if err := token.Error(); err != nil {
			s.close()
			return err
		}
	case <-ctx.Done():
		s.close()
		return ctx.Err()
	}
	if !s.raw.IsConnectionOpen() || sessionCtx.Err() != nil {
		s.close()
		return errors.New("MQTT 连接已断开")
	}
	slog.Info("已连接 MQTT Broker", "broker", c.broker)
	if notify && c.opts.OnConnect != nil {
		go c.opts.OnConnect(s.raw)
	}
	return nil
}

func (c *Client) connectionLost(s *connection, err error) {
	s.close() // No packet from this session can reach a future retained value.
	c.mu.Lock()
	if c.closed || c.current != s {
		c.mu.Unlock()
		return
	}
	c.ready = false
	c.mu.Unlock()
	s.lostOnce.Do(func() {
		select {
		case c.lost <- s:
		case <-c.ctx.Done():
		}
		slog.Warn("MQTT 连接已断开，等待新会话", "err", err)
		if c.opts.OnConnectionLost != nil {
			go c.opts.OnConnectionLost(s.raw, err)
		}
	})
}

func (c *Client) waitRetry() bool {
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	select {
	case <-c.ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
func (c *Client) reconnectLoop() {
	defer c.workers.Done()
	for {
		select {
		case <-c.ctx.Done():
			return
		case s := <-c.lost:
			c.mu.RLock()
			current := c.current == s
			c.mu.RUnlock()
			if !current {
				continue
			}
		}
		for c.waitRetry() {
			if err := c.connectOnce(true); err == nil {
				break
			} else {
				slog.Warn("MQTT 重连失败", "err", err)
			}
		}
	}
}

func (c *Client) Raw() paho.Client {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.raw
}
func (c *Client) publishClient() paho.Client {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.closed || (c.opts != nil && !c.ready) || c.raw == nil || !c.raw.IsConnectionOpen() || (c.current != nil && c.current.ctx.Err() != nil) {
		return nil
	}
	return c.raw
}
func (c *Client) subscriptionsReady(raw paho.Client) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.raw != raw || !raw.IsConnectionOpen() || (c.current != nil && c.current.ctx.Err() != nil) {
		return false
	}
	c.ready = true
	return true
}
func (c *Client) suspendPublishing(raw paho.Client) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.raw == raw {
		c.ready = false
	}
}
func (c *Client) abortPublish(raw paho.Client, err error) {
	c.mu.RLock()
	s := c.current
	c.mu.RUnlock()
	if s != nil && s.raw == raw {
		c.connectionLost(s, err)
	}
}
func (c *Client) Disconnect() {
	c.mu.Lock()
	c.closed, c.ready = true, false
	s := c.current
	c.mu.Unlock()
	if c.cancel != nil {
		c.cancel()
	}
	if s != nil {
		s.close()
	}
	c.workers.Wait()
}

type connection struct {
	mu        sync.Mutex
	ctx       context.Context
	cancel    context.CancelFunc
	raw       paho.Client
	transport net.Conn
	closeOnce sync.Once
	lostOnce  sync.Once
}

func (s *connection) close() {
	s.closeOnce.Do(func() {
		s.mu.Lock()
		s.cancel()
		if s.transport != nil {
			s.transport.Close()
		}
		s.mu.Unlock()
		// Discard Paho's tokens and worker state; this client is never reused.
		s.raw.Disconnect(0)
	})
}

type boundedConnection struct{ net.Conn }

func (c *boundedConnection) Write(p []byte) (int, error) {
	if err := c.Conn.SetWriteDeadline(time.Now().Add(mqttWriteTimeout)); err != nil {
		return 0, err
	}
	return c.Conn.Write(p)
}

// Track the transport so cancellation can close it before another session is
// opened. Keep the TCP/TLS/Unix/WebSocket schemes supported by Paho.
func dialMQTT(ctx context.Context, uri *url.URL, opts paho.ClientOptions) (net.Conn, error) {
	ctx, cancel := context.WithTimeout(ctx, opts.ConnectTimeout)
	defer cancel()
	dialer := &net.Dialer{Timeout: opts.ConnectTimeout}
	switch uri.Scheme {
	case "mqtt", "tcp":
		proxyDialer := proxy.FromEnvironmentUsing(dialer)
		if d, ok := proxyDialer.(proxy.ContextDialer); ok {
			return d.DialContext(ctx, "tcp", uri.Host)
		}
		return proxyDialer.Dial("tcp", uri.Host)
	case "unix":
		address := uri.Path
		if uri.Host != "" {
			address = uri.Host
		}
		return dialer.DialContext(ctx, "unix", address)
	case "ssl", "tls", "mqtts", "mqtt+ssl", "tcps":
		d := &tls.Dialer{NetDialer: dialer, Config: opts.TLSConfig}
		return d.DialContext(ctx, "tcp", uri.Host)
	case "ws", "wss":
		copyURI := *uri
		copyURI.User = nil
		return paho.NewWebsocket(copyURI.String(), opts.TLSConfig, opts.ConnectTimeout, opts.HTTPHeaders, opts.WebsocketOptions)
	default:
		return nil, fmt.Errorf("不支持的 MQTT 协议: %s", uri.Scheme)
	}
}
