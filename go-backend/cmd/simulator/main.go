// simulator publishes status/telemetry and acknowledges simulated device commands.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	paho "github.com/eclipse/paho.mqtt.golang"
	"net/url"
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
	configMode := flag.String("config-ack-mode", "success", "配置回执模式: success/failed/none")
	configDelay := flag.Duration("config-ack-delay", 0, "配置回执延迟")
	api := flag.String("api", "http://127.0.0.1:8080", "图片上传的后端 HTTP 根地址")
	eventImage := flag.Bool("event-image", false, "先发带 frame_id 的告警，再上传软件 event JPEG")
	mqttUser := flag.String("mqtt-username", os.Getenv("PIGUARD_SIM_MQTT_USERNAME"), "设备 MQTT 用户名（等于 device）")
	mqttPassword := flag.String("mqtt-password", "", "设备 MQTT 密码，推荐环境变量")
	httpUser := flag.String("http-username", os.Getenv("PIGUARD_SIM_HTTP_USERNAME"), "设备 HTTP 用户名（等于 device）")
	httpPassword := flag.String("http-password", "", "设备 HTTP 密码，推荐环境变量")
	allowAnonymous := flag.Bool("allow-anonymous", false, "显式使用本地开发 HTTP /自备匿名 Broker；正式部署禁止")
	flag.Parse()
	provided := map[string]bool{}
	flag.Visit(func(f *flag.Flag) { provided[f.Name] = true })
	if !provided["mqtt-password"] {
		*mqttPassword = os.Getenv("PIGUARD_SIM_MQTT_PASSWORD")
	}
	if !provided["http-password"] {
		*httpPassword = os.Getenv("PIGUARD_SIM_HTTP_PASSWORD")
	}
	if *duration <= 0 || *ackDelay < 0 || *configDelay < 0 {
		fmt.Fprintln(os.Stderr, "duration must be positive")
		os.Exit(1)
	}
	if err := run(*broker, *device, *duration, *ackMode, *ackDelay, *configMode, *configDelay, *api, *eventImage, simulator.Credentials{Username: *mqttUser, Password: *mqttPassword}, simulator.Credentials{Username: *httpUser, Password: *httpPassword}, *allowAnonymous); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(broker, id string, duration time.Duration, ackMode string, ackDelay time.Duration, configMode string, configDelay time.Duration, api string, eventImage bool, mqttCredentials, httpCredentials simulator.Credentials, allowAnonymous bool) error {
	u, err := url.Parse(broker)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "tcp" && u.Scheme != "ssl" && u.Scheme != "ws" && u.Scheme != "wss") {
		return fmt.Errorf("broker 必须为不含凭据的 MQTT 地址")
	}
	if err := mqttCredentials.Validate(id, allowAnonymous); err != nil {
		return fmt.Errorf("MQTT: %w", err)
	}
	if err := httpCredentials.Validate(id, allowAnonymous); err != nil {
		return fmt.Errorf("HTTP: %w", err)
	}
	executor, err := simulator.NewCommandExecutor(ackMode)
	if err != nil {
		return err
	}
	snapshots, err := simulator.NewSnapshotClient(api)
	if err != nil {
		return err
	}
	executor.SetCameraCapture(snapshots.Capture)
	if err := snapshots.SetCredentials(id, httpCredentials, allowAnonymous); err != nil {
		return err
	}
	configExecutor, err := simulator.NewConfigExecutor(configMode)
	if err != nil {
		return err
	}
	will := fmt.Sprintf(`{"schema_version":1,"device_id":%q,"online":false,"reason":"connection_lost"}`, id)
	opts := paho.NewClientOptions().AddBroker(broker).SetClientID(fmt.Sprintf("mock-%s-%d", id, time.Now().UnixNano())).SetConnectTimeout(3*time.Second).SetWill("car/"+id+"/status", will, 1, true)
	if mqttCredentials.Username != "" {
		opts.SetUsername(mqttCredentials.Username).SetPassword(mqttCredentials.Password)
	}
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
	type queuedAck struct {
		kind  string
		data  any
		delay time.Duration
	}
	ackQueue := make(chan queuedAck, 64)
	type queuedCommand struct {
		topic   string
		payload []byte
	}
	commandQueue := make(chan queuedCommand, 64)
	deviceStatus := func() map[string]any {
		return map[string]any{"schema_version": 1, "device_id": id, "online": true, "timestamp": time.Now().UTC(), "software_version": "0.1.0", "config_version": configExecutor.Version(id), "mode": "mock", "sensors": map[string]string{"distance": "ok", "camera": "ok", "gyro": "ok", "temperature": "ok"}, "mqtt": map[string]bool{"connected": true}}
	}
	ackDone := make(chan struct{})
	go func() {
		defer close(ackDone)
		for {
			select {
			case <-ackCtx.Done():
				return
			case ack := <-ackQueue:
				timer := time.NewTimer(ack.delay)
				select {
				case <-ackCtx.Done():
					timer.Stop()
					return
				case <-timer.C:
				}
				if err := publish(ack.kind, ack.data, 1, false); err != nil {
					fmt.Fprintln(os.Stderr, "command Ack:", err)
				}
				if ack.kind == "config-acks" {
					if err := publish("status", deviceStatus(), 1, true); err != nil {
						fmt.Fprintln(os.Stderr, "status:", err)
					}
				}
			}
		}
	}()
	commandDone := make(chan struct{})
	go func() {
		defer close(commandDone)
		for {
			select {
			case <-ackCtx.Done():
				return
			case command := <-commandQueue:
				if ackCtx.Err() != nil {
					return
				}
				ack, executed, err := executor.ExecuteContext(ackCtx, command.topic, command.payload, time.Now().UTC())
				if err != nil {
					fmt.Fprintln(os.Stderr, "拒绝命令:", err)
					continue
				}
				if executed {
					fmt.Printf("模拟执行命令，累计次数=%d\n", executor.Executions())
				}
				if ack == nil {
					continue
				}
				select {
				case ackQueue <- queuedAck{kind: "command-acks", data: *ack, delay: ackDelay}:
				case <-ackCtx.Done():
					return
				default:
					fmt.Fprintln(os.Stderr, "回执队列已满")
				}
			}
		}
	}()
	var eventImageDone chan struct{}
	onlinePublished := false
	defer func() {
		stop()
		cancelAcks()
		<-commandDone
		<-ackDone
		if eventImageDone != nil {
			<-eventImageDone
		}
		if onlinePublished {
			_ = publish("status", map[string]any{"schema_version": 1, "device_id": id, "online": false, "reason": "simulator_stopped"}, 1, true)
		}
	}()
	token = c.Subscribe("car/"+id+"/commands", 1, func(_ paho.Client, msg paho.Message) {
		select {
		case commandQueue <- queuedCommand{topic: msg.Topic(), payload: append([]byte(nil), msg.Payload()...)}:
		case <-ackCtx.Done():
		default:
			fmt.Fprintln(os.Stderr, "命令队列已满")
		}
	})
	if !token.WaitTimeout(3 * time.Second) {
		return fmt.Errorf("MQTT command subscription timeout")
	}
	if err := token.Error(); err != nil {
		return err
	}
	token = c.Subscribe("car/"+id+"/config", 1, func(_ paho.Client, msg paho.Message) {
		ack, _, err := configExecutor.Apply(msg.Topic(), msg.Payload(), time.Now().UTC())
		if err != nil {
			fmt.Fprintln(os.Stderr, "拒绝配置:", err)
			return
		}
		if ack == nil {
			return
		}
		select {
		case ackQueue <- queuedAck{kind: "config-acks", data: *ack, delay: configDelay}:
		case <-ackCtx.Done():
		default:
			fmt.Fprintln(os.Stderr, "回执队列已满")
		}
	})
	if !token.WaitTimeout(3 * time.Second) {
		return fmt.Errorf("MQTT config subscription timeout")
	}
	if err := token.Error(); err != nil {
		return err
	}
	status := deviceStatus()
	if err := publish("status", status, 1, true); err != nil {
		return err
	}
	onlinePublished = true
	eventID, eventAt := fmt.Sprintf("evt-sim-%d", time.Now().UnixNano()), time.Now().UTC()
	if eventImage {
		if err := simulator.PublishDemoImageEvent(publish, id, eventID, eventAt); err != nil {
			return err
		}
		eventImageDone = make(chan struct{})
		go func() {
			defer close(eventImageDone)
			if _, err := snapshots.UploadEvent(ackCtx, id, eventID, eventAt); err != nil {
				fmt.Fprintln(os.Stderr, "告警图片上传失败:", err)
			}
		}()
	} else if err := simulator.PublishDemoEvent(publish, id, eventID, eventAt); err != nil {
		return err
	}
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
