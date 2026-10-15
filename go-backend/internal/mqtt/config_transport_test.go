package mqtt

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"
	"github.com/eclipse/paho.mqtt.golang/packets"
	"piguard/go-backend/internal/apperr"
	"piguard/go-backend/internal/db"
	"piguard/go-backend/internal/protocol"
	"piguard/go-backend/internal/repo"
	"piguard/go-backend/internal/service"
)

// A packet-level peer over net.Pipe exercises the installed Paho client,
// including its inflight tokens, actual network writes and reconnect lifecycle.
// This is software transport evidence, not a real Broker integration check.
type wirePeer struct {
	net.Conn
	mu      sync.Mutex
	packets chan packets.ControlPacket
}

func newWirePeer(t *testing.T, conn net.Conn) *wirePeer {
	t.Helper()
	p := &wirePeer{Conn: conn, packets: make(chan packets.ControlPacket, 32)}
	t.Cleanup(func() { conn.Close() })
	go func() {
		defer close(p.packets)
		for {
			packet, err := packets.ReadPacket(conn)
			if err != nil {
				return
			}
			p.packets <- packet
			switch packet.(type) {
			case *packets.ConnectPacket:
				ack := packets.NewControlPacket(packets.Connack).(*packets.ConnackPacket)
				if p.write(ack) != nil {
					return
				}
			case *packets.PingreqPacket:
				if p.write(packets.NewControlPacket(packets.Pingresp)) != nil {
					return
				}
			}
		}
	}()
	return p
}
func (p *wirePeer) write(packet packets.ControlPacket) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.Conn.SetWriteDeadline(time.Now().Add(time.Second)); err != nil {
		return err
	}
	return packet.Write(p.Conn)
}
func (p *wirePeer) next(t *testing.T) packets.ControlPacket {
	t.Helper()
	select {
	case packet, ok := <-p.packets:
		if !ok {
			t.Fatal("MQTT transport closed before expected packet")
		}
		return packet
	case <-time.After(6 * time.Second):
		t.Fatal("missing MQTT packet")
	}
	return nil
}
func (p *wirePeer) subscribe(t *testing.T) *packets.SubscribePacket {
	t.Helper()
	packet, ok := p.next(t).(*packets.SubscribePacket)
	if !ok {
		t.Fatal("first packet after CONNECT must be SUBSCRIBE")
	}
	if len(packet.Topics) != len(uplinkFilters) {
		t.Fatal(packet.Topics)
	}
	for i, topic := range packet.Topics {
		qos, exists := uplinkFilters[topic]
		if !exists || qos != packet.Qoss[i] {
			t.Fatal("incorrect uplink subscription", packet)
		}
	}
	return packet
}
func (p *wirePeer) suback(t *testing.T, sub *packets.SubscribePacket) {
	t.Helper()
	ack := packets.NewControlPacket(packets.Suback).(*packets.SubackPacket)
	ack.MessageID, ack.ReturnCodes = sub.MessageID, sub.Qoss
	if err := p.write(ack); err != nil {
		t.Fatal(err)
	}
}
func (p *wirePeer) puback(t *testing.T, pub *packets.PublishPacket) {
	t.Helper()
	ack := packets.NewControlPacket(packets.Puback).(*packets.PubackPacket)
	ack.MessageID = pub.MessageID
	if err := p.write(ack); err != nil {
		t.Fatal(err)
	}
}
func (p *wirePeer) config(t *testing.T, version int) *packets.PublishPacket {
	t.Helper()
	packet, ok := p.next(t).(*packets.PublishPacket)
	if !ok || packet.TopicName != "car/car-001/config" || packet.Qos != 1 || !packet.Retain || packet.Dup {
		t.Fatal("unexpected config packet", packet)
	}
	message, err := protocol.ParseConfigMessage(packet.TopicName, packet.Payload)
	if err != nil || message.ConfigVersion != version {
		t.Fatal(message, err)
	}
	return packet
}
func (p *wirePeer) quiet(t *testing.T) {
	t.Helper()
	select {
	case packet, ok := <-p.packets:
		t.Fatal("unexpected packet or closed transport", packet, ok)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestMQTTReconnectDiscardsInflightBeforeLatestRestore(t *testing.T) {
	peers := make(chan *wirePeer, 4)
	client := NewClient(Options{Broker: "tcp://software-peer:1883", ClientID: "transport", ConnectTimeout: time.Second})
	client.opts.SetCustomOpenConnectionFn(func(_ *url.URL, _ paho.ClientOptions) (net.Conn, error) {
		local, remote := net.Pipe()
		peers <- newWirePeer(t, remote)
		return local, nil
	})
	t.Cleanup(client.Disconnect)
	publisher := NewPublisher(client)
	g, err := db.Open(filepath.Join(t.TempDir(), "configs.db"))
	if err != nil {
		t.Fatal(err)
	}
	sql, _ := g.DB()
	t.Cleanup(func() { sql.Close() })
	configs, devices := repo.NewConfigRepository(g), repo.NewDeviceRepository(g)
	if _, err := service.NewDeviceService(devices, configs).Seed(context.Background(), service.SeedInput{DeviceID: "car-001", Name: "Demo", Mode: "mock", SoftwareVersion: "0.1.0"}); err != nil {
		t.Fatal(err)
	}
	manager := service.NewConfigService(configs, publisher, devices)
	if err := manager.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Close)
	subscriber := NewSubscriber(client, nil, nil, nil, manager)
	t.Cleanup(subscriber.Close)
	if err := client.Connect(); err != nil {
		t.Fatal(err)
	}
	first := <-peers
	if connect, ok := first.next(t).(*packets.ConnectPacket); !ok || !connect.CleanSession {
		t.Fatal("expected a clean session", connect)
	}
	if publisher.Available() {
		t.Fatal("publishing available before SUBACK")
	}
	started := make(chan error, 1)
	go func() { started <- subscriber.Start() }()
	sub := first.subscribe(t)
	first.quiet(t)
	first.suback(t, sub)
	if err := <-started; err != nil {
		t.Fatal(err)
	}
	first.puback(t, first.config(t, 1))

	// Keep both an action and v2 unacknowledged. v3 is committed while the
	// v2 sender waits. The publication deadline must close this transport;
	// reconnection must discard both old inflight packets.
	commandDone := make(chan error, 1)
	go func() {
		commandDone <- publisher.Publish(context.Background(), "car/car-001/commands", 1, false, []byte(`{"command_id":"old-action"}`))
	}()
	if packet, ok := first.next(t).(*packets.PublishPacket); !ok || packet.TopicName != "car/car-001/commands" {
		t.Fatal("missing old action", packet)
	}
	update := func(expected int, patch string) {
		t.Helper()
		if version, err := manager.Update(context.Background(), "car-001", protocol.ConfigRequest{ExpectedVersion: expected, Rules: json.RawMessage(patch)}); err != nil || version != expected+1 {
			t.Fatal(version, err)
		}
	}
	update(1, `{"obstacle":{"warning_distance_m":20}}`)
	first.config(t, 2) // Intentionally do not send PUBACK.
	update(2, `{"obstacle":{"warning_distance_m":25}}`)
	if err := <-commandDone; !errors.Is(err, apperr.ErrPublishUncertain) {
		t.Fatal("unacknowledged action must remain uncertain", err)
	}
	second := <-peers
	if connect, ok := second.next(t).(*packets.ConnectPacket); !ok || !connect.CleanSession {
		t.Fatal("reconnect did not create a clean session", connect)
	}
	if publisher.Available() {
		t.Fatal("reconnect available before subscriptions restored")
	}
	sub = second.subscribe(t)
	second.quiet(t) // No old config or action replay while SUBACK is held.
	second.suback(t, sub)
	latest := second.config(t, 3)
	second.puback(t, latest)
	second.quiet(t) // Neither v2 nor the action may follow retained v3.
	row, err := configs.Snapshot(context.Background(), "car-001")
	if err != nil || row.Config.DesiredVersion != 3 || row.Revision.Status != "pending" {
		t.Fatal(row, err)
	}
	var message protocol.ConfigMessage
	if err := json.Unmarshal(latest.Payload, &message); err != nil || !message.IssuedAt.Equal(row.Revision.IssuedAt) || message.Rules.Obstacle.WarningDistanceM != 25 {
		t.Fatal("restore changed the issued version", message, row, err)
	}
	at := time.Now().UTC()
	ack, _ := json.Marshal(protocol.ConfigAck{SchemaVersion: 1, DeviceID: "car-001", ConfigVersion: 3, Status: "success", AppliedAt: &at})
	packet := packets.NewControlPacket(packets.Publish).(*packets.PublishPacket)
	packet.TopicName, packet.Qos, packet.MessageID, packet.Payload = "car/car-001/config-acks", 1, 900, ack
	if err := second.write(packet); err != nil {
		t.Fatal(err)
	}
	if received, ok := second.next(t).(*packets.PubackPacket); !ok || received.MessageID != 900 {
		t.Fatal("config Ack subscription unavailable", received)
	}
	view, err := manager.Get(context.Background(), "car-001")
	if err != nil || view.Status != "success" || view.ReportedVersion != 3 {
		t.Fatal(view, err)
	}
}

func TestMQTTCancellationClosesBlockedTransportAndStopsReconnect(t *testing.T) {
	client := NewClient(Options{Broker: "tcp://software-peer:1883", ClientID: "blocked", ConnectTimeout: time.Second})
	var opens atomic.Int32
	var peer net.Conn
	client.opts.SetCustomOpenConnectionFn(func(_ *url.URL, _ paho.ClientOptions) (net.Conn, error) {
		opens.Add(1)
		local, remote := net.Pipe()
		peer = remote
		go func() {
			packet, err := packets.ReadPacket(remote)
			if err != nil {
				return
			}
			if _, ok := packet.(*packets.ConnectPacket); !ok {
				return
			}
			packets.NewControlPacket(packets.Connack).Write(remote)
			// Never read PUBLISH: exercise a blocked actual Paho Write/Publish.
		}()
		return local, nil
	})
	t.Cleanup(client.Disconnect)
	if err := client.Connect(); err != nil {
		t.Fatal(err)
	}
	defer peer.Close()
	if !client.subscriptionsReady(client.Raw()) {
		t.Fatal("test session did not become ready")
	}
	publisher := NewPublisher(client)
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	before := time.Now()
	if err := publisher.Publish(ctx, "car/car-001/config", 1, true, []byte(`{}`)); !errors.Is(err, apperr.ErrPublishUncertain) {
		t.Fatal(err)
	}
	if time.Since(before) > 500*time.Millisecond || publisher.Available() {
		t.Fatal("cancellation failed to close blocked session promptly")
	}
	before = time.Now()
	client.Disconnect()
	if time.Since(before) > 500*time.Millisecond || opens.Load() != 1 {
		t.Fatal("close waited or reopened the connection", opens.Load())
	}
	peer.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	if _, err := packets.ReadPacket(peer); err == nil {
		t.Fatal("abandoned transport still sent a packet")
	}
}

func TestMQTTRejectedConfigAckSubscriptionPreventsRestore(t *testing.T) {
	peers := make(chan *wirePeer, 1)
	client := NewClient(Options{Broker: "tcp://software-peer:1883", ClientID: "rejected", ConnectTimeout: time.Second})
	client.opts.SetCustomOpenConnectionFn(func(_ *url.URL, _ paho.ClientOptions) (net.Conn, error) {
		local, remote := net.Pipe()
		peers <- newWirePeer(t, remote)
		return local, nil
	})
	t.Cleanup(client.Disconnect)
	configs := &countingConfigSync{restored: make(chan struct{}, 1)}
	subscriber := NewSubscriber(client, nil, nil, nil, configs)
	t.Cleanup(subscriber.Close)
	if err := client.Connect(); err != nil {
		t.Fatal(err)
	}
	peer := <-peers
	peer.next(t) // CONNECT
	started := make(chan error, 1)
	go func() { started <- subscriber.Start() }()
	sub := peer.subscribe(t)
	ack := packets.NewControlPacket(packets.Suback).(*packets.SubackPacket)
	ack.MessageID, ack.ReturnCodes = sub.MessageID, append([]byte(nil), sub.Qoss...)
	for i, topic := range sub.Topics {
		if topic == "car/+/config-acks" {
			ack.ReturnCodes[i] = 0x80
		}
	}
	if err := peer.write(ack); err != nil {
		t.Fatal(err)
	}
	if err := <-started; err == nil || NewPublisher(client).Available() || subscriber.started.Load() {
		t.Fatal("rejected subscription was considered ready", err)
	}
	select {
	case <-configs.restored:
		t.Fatal("configuration restored without its Ack subscription")
	default:
	}
	peer.quiet(t)
}
