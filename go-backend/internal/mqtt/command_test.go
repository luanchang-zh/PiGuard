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
	"piguard/go-backend/internal/protocol"
	"piguard/go-backend/internal/repo"
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
	handler       paho.MessageHandler
}

func (c *testClient) IsConnectionOpen() bool { return c.open.Load() }
func (c *testClient) Publish(string, byte, bool, interface{}) paho.Token {
	c.publishes.Add(1)
	return c.token
}
func (c *testClient) SubscribeMultiple(filters map[string]byte, handler paho.MessageHandler) paho.Token {
	c.handler = handler
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
	s := NewSubscriber(c, nil, nil, nil)
	defer s.Close()
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	check := func() {
		t.Helper()
		select {
		case topics := <-raw.subscriptions:
			if len(topics) != 5 || topics["car/+/config-acks"] != 1 || topics["car/+/command-acks"] != 1 || topics["car/+/status"] != 1 || topics["car/+/telemetry"] != 0 || topics["car/+/events"] != 1 {
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
	s = NewSubscriber(failedClient, nil, nil, nil)
	defer s.Close()
	if err := s.Start(); err == nil || s.started.Load() {
		t.Fatal("initial subscription failure hidden", err)
	}
}

type countingEvents struct{ n atomic.Int32 }

func (c *countingEvents) Ingest(context.Context, string, []byte) error {
	c.n.Add(1)
	return nil
}
func (c *countingEvents) List(context.Context, string, repo.EventQuery) ([]protocol.EventItem, error) {
	return []protocol.EventItem{}, nil
}

type testMessage struct {
	topic   string
	payload []byte
}

func (m testMessage) Duplicate() bool   { return false }
func (m testMessage) Qos() byte         { return 1 }
func (m testMessage) Retained() bool    { return false }
func (m testMessage) Topic() string     { return m.topic }
func (m testMessage) MessageID() uint16 { return 1 }
func (m testMessage) Payload() []byte   { return m.payload }
func (m testMessage) Ack()              {}

func TestSubscriberRoutesEventsWithoutBlockingAckRecovery(t *testing.T) {
	raw := &testClient{token: completedToken(nil), subscriptions: make(chan map[string]byte, 1)}
	raw.open.Store(true)
	client := NewClient(Options{Broker: "tcp://127.0.0.1:1883", ClientID: "events", ConnectTimeout: time.Second})
	client.raw = raw
	events := &countingEvents{}
	s := NewSubscriber(client, nil, nil, events)
	defer s.Close()
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	<-raw.subscriptions
	raw.handler(nil, testMessage{topic: "car/car-001/events", payload: []byte(`{}`)})
	if events.n.Load() != 1 {
		t.Fatal(events.n.Load())
	}
}

type countingConfigSync struct {
	acks     atomic.Int32
	restored chan struct{}
	raw      *testClient
}

func (c *countingConfigSync) IngestAck(context.Context, string, []byte) error {
	c.acks.Add(1)
	return nil
}
func (c *countingConfigSync) Restore(context.Context) error {
	if c.raw.handler == nil {
		return errors.New("restore preceded subscription")
	}
	c.restored <- struct{}{}
	return nil
}
func TestSubscriberConfigAckRoutingAndRestoreAfterSubscription(t *testing.T) {
	raw := &testClient{token: completedToken(nil), subscriptions: make(chan map[string]byte, 4)}
	raw.open.Store(true)
	client := NewClient(Options{Broker: "tcp://127.0.0.1:1883", ClientID: "config", ConnectTimeout: time.Second})
	client.raw = raw
	configs := &countingConfigSync{raw: raw, restored: make(chan struct{}, 4)}
	s := NewSubscriber(client, nil, nil, nil, configs)
	defer s.Close()
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	<-configs.restored
	topics := <-raw.subscriptions
	if len(topics) != 5 || topics["car/+/config-acks"] != 1 {
		t.Fatal(topics)
	}
	raw.handler(nil, testMessage{topic: "car/car-001/config-acks", payload: []byte(`{}`)})
	if configs.acks.Load() != 1 {
		t.Fatal("config Ack not routed")
	}
	client.opts.OnConnect(raw)
	select {
	case <-configs.restored:
	case <-time.After(time.Second):
		t.Fatal("reconnect did not restore configs")
	}
	topics = <-raw.subscriptions
	if len(topics) != 5 {
		t.Fatal(topics)
	}
	s.Close()
	client.opts.OnConnect(raw)
	select {
	case <-configs.restored:
		t.Fatal("closed subscriber restored configuration")
	default:
	}
}
