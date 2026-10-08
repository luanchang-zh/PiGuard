// simulator publishes status/telemetry and acknowledges simulated device commands.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	paho "github.com/eclipse/paho.mqtt.golang"
	"os"
	"os/signal"
	"piguard/go-backend/internal/protocol"
	"piguard/go-backend/internal/simulator"
	"syscall"
	"time"
)

func ptr[T any](value T) *T { return &value }

func main() {
	broker := flag.String("broker", "tcp://127.0.0.1:1883", "MQTT Broker")
	device := flag.String("device", "car-001", "已登记设备 ID")
	duration := flag.Duration("duration", 10*time.Second, "模拟持续时间")
	ackMode := flag.String("ack-mode", "success", "命令回执模式: success/failed/none")
	ackDelay := flag.Duration("ack-delay", 0, "回执延迟，用于验证迟到和超时")
	flag.Parse()
	if *duration <= 0 || *ackDelay < 0 {
		fmt.Fprintln(os.Stderr, "duration must be positive")
		os.Exit(1)
	}
	if err := run(*broker, *device, *duration, *ackMode, *ackDelay); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(broker, id string, duration time.Duration, ackMode string, ackDelay time.Duration) error {
	executor, err := simulator.NewCommandExecutor(ackMode)
	if err != nil {
		return err
	}
	will := fmt.Sprintf(`{"schema_version":1,"device_id":%q,"online":false,"reason":"connection_lost"}`, id)
	opts := paho.NewClientOptions().AddBroker(broker).SetClientID(fmt.Sprintf("mock-%s-%d", id, time.Now().UnixNano())).SetConnectTimeout(3*time.Second).SetWill("car/"+id+"/status", will, 1, true)
	c := paho.NewClient(opts)
	token := c.Connect()
	if !token.WaitTimeout(5 * time.Second) {
		return fmt.Errorf("MQTT connect timeout")
	}
	if token.Error() != nil {
		return token.Error()
	}
	defer c.Disconnect(500)
	publish := func(kind string, data any, qos byte, retained bool) error {
		raw, err := json.Marshal(data)
		if err != nil {
			return err
		}
		token := c.Publish("car/"+id+"/"+kind, qos, retained, raw)
		if !token.WaitTimeout(3 * time.Second) {
			return fmt.Errorf("MQTT publish timeout")
		}
		return token.Error()
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	ackCtx, cancelAcks := context.WithCancel(ctx)
	ackQueue := make(chan protocol.CommandAck, 64)
	ackDone := make(chan struct{})
	go func() {
		defer close(ackDone)
		for {
			select {
			case <-ackCtx.Done():
				return
			case ack := <-ackQueue:
				timer := time.NewTimer(ackDelay)
				select {
				case <-ackCtx.Done():
					timer.Stop()
					return
				case <-timer.C:
				}
				if err := publish("command-acks", ack, 1, false); err != nil {
					fmt.Fprintln(os.Stderr, "command Ack:", err)
				}
			}
		}
	}()
	defer func() { stop(); cancelAcks(); <-ackDone }()
	token = c.Subscribe("car/"+id+"/commands", 1, func(_ paho.Client, msg paho.Message) {
		ack, executed, err := executor.Execute(msg.Topic(), msg.Payload(), time.Now().UTC())
		if err != nil {
			fmt.Fprintln(os.Stderr, "拒绝命令:", err)
			return
		}
		if executed {
			fmt.Printf("模拟执行命令，累计次数=%d\n", executor.Executions())
		}
		if ack == nil {
			return
		}
		select {
		case ackQueue <- *ack:
		case <-ackCtx.Done():
		default:
			fmt.Fprintln(os.Stderr, "回执队列已满")
		}
	})
	if !token.WaitTimeout(3 * time.Second) {
		return fmt.Errorf("MQTT command subscription timeout")
	}
	if err := token.Error(); err != nil {
		return err
	}
	status := map[string]any{"schema_version": 1, "device_id": id, "online": true, "timestamp": time.Now().UTC(), "software_version": "0.1.0", "config_version": 0, "mode": "mock", "sensors": map[string]string{"distance": "ok", "camera": "ok", "gyro": "ok", "temperature": "ok"}, "mqtt": map[string]bool{"connected": true}}
	if err := publish("status", status, 1, true); err != nil {
		return err
	}
	defer func() {
		cancelAcks()
		<-ackDone
		_ = publish("status", map[string]any{"schema_version": 1, "device_id": id, "online": false, "reason": "simulator_stopped"}, 1, true)
	}()
	timer := time.NewTimer(duration)
	defer timer.Stop()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	fmt.Printf("发布 %s 的在线状态和 2Hz 遥测，持续 %s\n", id, duration)
	for seq := uint64(0); ; seq++ {
		at := time.Now().UTC()
		t := protocol.Telemetry{SchemaVersion: 1, DeviceID: id, MessageID: fmt.Sprintf("mock-%d", seq), Timestamp: at, Seq: &seq,
			Speed:       protocol.Sample{Value: ptr(32.4), Unit: "km/h", Source: "mock", Status: "ok", SampleAt: &at},
			Distance:    protocol.Distance{RawValue: ptr(.18), MappedValue: ptr(18.0), Scale: ptr(100.0), Unit: "m", Source: "mock", Status: "ok", SampleAt: &at},
			Temperature: protocol.Sample{Value: ptr(27.5), Unit: "C", Source: "mock", Status: "ok", SampleAt: &at},
			Gyro:        protocol.Gyro{X: ptr(.6), Y: ptr(1.2), Z: ptr(12.8), YawRate: ptr(12.8), Unit: "deg/s", Source: "mock", Status: "ok", SampleAt: &at},
			Lane:        protocol.Lane{Valid: true, OffsetRatio: ptr(.18), Confidence: ptr(.91), Direction: "right", SampleAt: &at}, Risk: protocol.Risk{Level: "normal", ActiveEvents: []string{}}}
		if err := publish("telemetry", t, 0, false); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return nil
		case <-timer.C:
			return nil
		case <-ticker.C:
		}
	}
}
