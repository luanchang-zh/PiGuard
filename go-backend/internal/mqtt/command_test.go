package mqtt

import (
	"context"
	"errors"
	"maps"
	"sync/atomic"
	"testing"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"
	"piguard/go-backend/internal/apperr"
)

type testToken struct {
	done chan struct{}
	err  error
}

func (t *testToken) Wait() bool { <-t.done; return true }
func (t *testToken) WaitTimeout(d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-t.done:
		return true
	case <-timer.C:
		return false
	}
}
func (t *testToken) Done() <-chan struct{} { return t.done }
func (t *testToken) Error() error          { return t.err }
func completedToken(err error) *testToken {
	t := &testToken{done: make(chan struct{}), err: err}
	close(t.done)
	return t
}

type testClient struct {
	paho.Client
	open          atomic.Bool
	publishes     atomic.Int64
	token         paho.Token
	subscriptions chan map[string]byte
}

func (c *testClient) IsConnectionOpen() bool { return c.open.Load() }
func (c *testClient) Publish(string, byte, bool, interface{}) paho.Token {
	c.publishes.Add(1)
	return c.token
}
func (c *testClient) SubscribeMultiple(filters map[string]byte, _ paho.MessageHandler) paho.Token {
	c.subscriptions <- maps.Clone(filters)
	return c.token
}

func TestPublisherAcknowledgementAndUncertainDelivery(t *testing.T) {
	for _, tc := range []struct {
		name  string
		token paho.Token
		want  error
	}{{"accepted", completedToken(nil), nil}, {"token error", completedToken(errors.New("connection lost")), apperr.ErrPublishUncertain}, {"deadline", &testToken{done: make(chan struct{})}, apperr.ErrPublishUncertain}} {
		t.Run(tc.name, func(t *testing.T) {
			c := &testClient{token: tc.token}
			c.open.Store(true)
			p := NewPublisher(&Client{raw: c})
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
			defer cancel()
			err := p.Publish(ctx, "car/car-001/commands", 1, false, []byte(`{}`))
			if !errors.Is(err, tc.want) {
				t.Fatal(err)
			}
			if c.publishes.Load() != 1 {
				t.Fatal("message not submitted once")
			}
		})
	}
	c := &testClient{token: completedToken(nil)}
	p := NewPublisher(&Client{raw: c})
	if p.Available() {
		t.Fatal("disconnected available")
	}
	if err := p.Publish(context.Background(), "topic", 1, false, nil); !errors.Is(err, apperr.ErrMQTTUnavailable) {
		t.Fatal(err)
	}
	c.open.Store(true)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := p.Publish(ctx, "topic", 1, false, nil); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if c.publishes.Load() != 0 {
		t.Fatal("refused publish reached client")
	}
}

func TestSubscriberRestoresAckTopicOnReconnect(t *testing.T) {
	raw := &testClient{token: completedToken(nil), subscriptions: make(chan map[string]byte, 4)}
	raw.open.Store(true)
	c := NewClient(Options{Broker: "tcp://127.0.0.1:1883", ClientID: "test", ConnectTimeout: time.Second})
	c.raw = raw
	s := NewSubscriber(c, nil, nil)
	defer s.Close()
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	check := func() {
		t.Helper()
		select {
		case topics := <-raw.subscriptions:
			if len(topics) != 3 || topics["car/+/command-acks"] != 1 || topics["car/+/status"] != 1 || topics["car/+/telemetry"] != 0 {
				t.Fatal(topics)
			}
		case <-time.After(time.Second):
			t.Fatal("missing subscription")
		}
	}
	check()
	c.opts.OnConnect(raw)
	check()
	s.Close()
	c.opts.OnConnect(raw)
	select {
	case <-raw.subscriptions:
		t.Fatal("closed subscriber restored topics")
	default:
	}
	failedRaw := &testClient{token: completedToken(errors.New("subscription rejected")), subscriptions: make(chan map[string]byte, 1)}
	failedClient := NewClient(Options{Broker: "tcp://127.0.0.1:1883", ClientID: "failure", ConnectTimeout: time.Second})
	failedClient.raw = failedRaw
	s = NewSubscriber(failedClient, nil, nil)
	defer s.Close()
	if err := s.Start(); err == nil || s.started.Load() {
		t.Fatal("initial subscription failure hidden", err)
	}
}
