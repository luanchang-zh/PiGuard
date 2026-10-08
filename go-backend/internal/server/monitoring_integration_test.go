//go:build integration

package server_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	paho "github.com/eclipse/paho.mqtt.golang"
	"github.com/gorilla/websocket"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"piguard/go-backend/internal/config"
	"piguard/go-backend/internal/protocol"
	"piguard/go-backend/internal/server"
	"piguard/go-backend/internal/service"
	"piguard/go-backend/internal/testutil"
	"sync"
	"testing"
	"time"
)

// The helper executes the actual server lifecycle in its own process so that
// its signal handlers do not interfere with the test runner.
func TestServerProcess(t *testing.T) {
	if os.Getenv("PIGUARD_TEST_SERVER") != "1" {
		return
	}
	cfg, err := config.Load(os.Getenv("PIGUARD_TEST_CONFIG"))
	if err != nil {
		t.Fatal(err)
	}
	if err := server.Run(cfg); err != nil {
		t.Fatal(err)
	}
}

type safeLog struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (l *safeLog) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.Write(p)
}
func (l *safeLog) String() string { l.mu.Lock(); defer l.mu.Unlock(); return l.buf.String() }

type process struct {
	cmd  *exec.Cmd
	done chan error
	once sync.Once
	log  *safeLog
	err  error
}

func start(t *testing.T, cmd *exec.Cmd) *process {
	t.Helper()
	p := &process{cmd: cmd, done: make(chan error, 1), log: &safeLog{}}
	cmd.Stdout, cmd.Stderr = p.log, p.log
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go func() { p.done <- cmd.Wait() }()
	t.Cleanup(func() {
		if err := p.stop(); err != nil {
			t.Errorf("subprocess %s failed: %v", cmd.Path, err)
		}
		if t.Failed() {
			t.Log(p.log.String())
		}
	})
	return p
}
func (p *process) stop() error {
	p.once.Do(func() {
		_ = p.cmd.Process.Signal(os.Interrupt)
		select {
		case err := <-p.done:
			p.err = err
		case <-time.After(6 * time.Second):
			_ = p.cmd.Process.Kill()
			p.err = <-p.done
		}
	})
	return p.err
}
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}
func eventually(t *testing.T, what string, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(12 * time.Second)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("timed out: " + what)
}

func readData(url string, dst any) error {
	client := http.Client{Timeout: time.Second}
	response, err := client.Get(url)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return fmt.Errorf("HTTP %d", response.StatusCode)
	}
	var envelope struct {
		Code int             `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		return err
	}
	if envelope.Code != 0 {
		return fmt.Errorf("code %d", envelope.Code)
	}
	return json.Unmarshal(envelope.Data, dst)
}
func publisher(t *testing.T, port int) paho.Client {
	t.Helper()
	c := paho.NewClient(paho.NewClientOptions().AddBroker(fmt.Sprintf("tcp://127.0.0.1:%d", port)).SetClientID(fmt.Sprintf("integration-%d", time.Now().UnixNano())).SetConnectTimeout(2 * time.Second))
	token := c.Connect()
	if !token.WaitTimeout(3*time.Second) || token.Error() != nil {
		t.Fatal("MQTT publisher connect:", token.Error())
	}
	t.Cleanup(func() { c.Disconnect(100) })
	return c
}
func publish(t *testing.T, c paho.Client, kind string, value any) {
	t.Helper()
	qos := byte(0)
	retain := false
	if kind == "status" {
		qos = 1
		retain = true
	}
	token := c.Publish("car/car-001/"+kind, qos, retain, testutil.JSON(value))
	if !token.WaitTimeout(3*time.Second) || token.Error() != nil {
		t.Fatal("publish:", token.Error())
	}
}

type wsEvent struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

func websocketReader(t *testing.T, url string) (*websocket.Conn, <-chan wsEvent) {
	t.Helper()
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	events := make(chan wsEvent, 256)
	go func() {
		defer close(events)
		for {
			var e wsEvent
			if err := conn.ReadJSON(&e); err != nil {
				return
			}
			select {
			case events <- e:
			default:
				return
			}
		}
	}()
	return conn, events
}
func waitEvent(t *testing.T, ch <-chan wsEvent, match func(wsEvent) bool) {
	t.Helper()
	timer := time.NewTimer(4 * time.Second)
	defer timer.Stop()
	for {
		select {
		case e, ok := <-ch:
			if !ok {
				t.Fatal("WS closed")
			}
			if match(e) {
				return
			}
		case <-timer.C:
			t.Fatal("missing WS event")
		}
	}
}

func TestMonitoringBrokerLifecycle(t *testing.T) {
	bin := os.Getenv("MOSQUITTO_BIN")
	if bin == "" {
		t.Fatal("MOSQUITTO_BIN is required for real Broker verification")
	}
	root := t.TempDir()
	brokerPort, httpPort := freePort(t), freePort(t)
	brokerConfig := filepath.Join(root, "mosquitto.conf")
	if err := os.WriteFile(brokerConfig, []byte(fmt.Sprintf("listener %d 127.0.0.1\nallow_anonymous true\npersistence false\nlog_dest stderr\n", brokerPort)), 0600); err != nil {
		t.Fatal(err)
	}
	startBroker := func() *process {
		p := start(t, exec.Command(bin, "-c", brokerConfig))
		eventually(t, "Broker listen", func() bool {
			c, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", brokerPort), 100*time.Millisecond)
			if err != nil {
				return false
			}
			_ = c.Close()
			return true
		})
		return p
	}
	broker := startBroker()
	backendConfig := filepath.Join(root, "backend.yaml")
	yaml := fmt.Sprintf("http:\n  addr: 127.0.0.1:%d\nmqtt:\n  broker: tcp://127.0.0.1:%d\n  client_id: backend-integration\n  connect_timeout_seconds: 2\ndatabase:\n  sqlite_path: %s\nstorage:\n  snapshot_dir: %s\nseed:\n  device_id: car-001\n  name: Demo Car\n  mode: mock\n  software_version: 0.1.0\n", httpPort, brokerPort, filepath.Join(root, "data.db"), filepath.Join(root, "snapshots"))
	if err := os.WriteFile(backendConfig, []byte(yaml), 0600); err != nil {
		t.Fatal(err)
	}
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", httpPort)
	stateURL := baseURL + "/api/v1/devices/car-001/state"
	startBackend := func() *process {
		cmd := exec.Command(os.Args[0], "-test.run=^TestServerProcess$")
		cmd.Env = append(os.Environ(), "PIGUARD_TEST_SERVER=1", "PIGUARD_TEST_CONFIG="+backendConfig)
		p := start(t, cmd)
		eventually(t, "backend health", func() bool {
			r, err := http.Get(baseURL + "/health")
			if err != nil {
				return false
			}
			defer r.Body.Close()
			_, _ = io.Copy(io.Discard, r.Body)
			return r.StatusCode == 200
		})
		return p
	}
	backend := startBackend()
	var state protocol.State
	if err := readData(stateURL, &state); err != nil || state.Timestamp != nil || state.Online {
		t.Fatal("bad startup state", state, err)
	}
	wsURL := fmt.Sprintf("ws://127.0.0.1:%d/api/v1/ws", httpPort)
	a, first := websocketReader(t, wsURL)
	_, second := websocketReader(t, wsURL)
	pub := publisher(t, brokerPort)
	publish(t, pub, "status", testutil.Status("car-001", time.Now(), true))
	eventually(t, "online status", func() bool { return readData(stateURL, &state) == nil && state.Online })
	waitEvent(t, first, func(e wsEvent) bool { return e.Type == "device_status" })
	waitEvent(t, second, func(e wsEvent) bool { return e.Type == "device_status" })
	var latest time.Time
	ticker := time.NewTicker(500 * time.Millisecond)
	for i := 0; i < 20; i++ {
		if i > 0 {
			<-ticker.C
		}
		latest = time.Now().UTC()
		publish(t, pub, "telemetry", testutil.Telemetry("car-001", latest, uint64(i)))
	}
	ticker.Stop()
	eventually(t, "latest telemetry", func() bool {
		return readData(stateURL, &state) == nil && state.Timestamp != nil && state.Timestamp.Equal(latest)
	})
	if state.Distance == nil || *state.Distance != 18 || state.Samples["distance"].SampleAt == nil {
		t.Fatal("bad real mapping", state)
	}
	for _, ch := range []<-chan wsEvent{first, second} {
		waitEvent(t, ch, func(e wsEvent) bool {
			var s protocol.State
			return e.Type == "telemetry" && json.Unmarshal(e.Data, &s) == nil && s.Timestamp != nil && s.Timestamp.Equal(latest)
		})
	}
	var history struct {
		Items []service.HistoryItem `json:"items"`
	}
	if err := readData(baseURL+"/api/v1/devices/car-001/telemetry?limit=100", &history); err != nil || len(history.Items) < 5 || len(history.Items) > 10 {
		t.Fatal("1Hz history", len(history.Items), err)
	}
	publish(t, pub, "telemetry", testutil.Telemetry("car-001", latest.Add(-time.Second), 100))
	time.Sleep(100 * time.Millisecond)
	if err := readData(stateURL, &state); err != nil || !state.Timestamp.Equal(latest) {
		t.Fatal("old MQTT message replaced latest")
	}
	latest = time.Now().UTC()
	publish(t, pub, "telemetry", testutil.Telemetry("car-001", latest, 0))
	eventually(t, "seq reset", func() bool {
		return readData(stateURL, &state) == nil && state.Timestamp != nil && state.Timestamp.Equal(latest)
	})
	waitEvent(t, second, func(e wsEvent) bool {
		var s protocol.State
		return e.Type == "telemetry" && json.Unmarshal(e.Data, &s) == nil && s.Timestamp != nil && s.Timestamp.Equal(latest) && s.Freshness["distance"] == "stale"
	})
	_ = a.Close()
	publish(t, pub, "status", testutil.Status("car-001", time.Now(), false))
	eventually(t, "timestamp-free will", func() bool { return readData(stateURL, &state) == nil && !state.Online })
	waitEvent(t, second, func(e wsEvent) bool {
		var s struct {
			Online bool `json:"online"`
		}
		return e.Type == "device_status" && json.Unmarshal(e.Data, &s) == nil && !s.Online
	})
	backend.stop()
	backend = startBackend()
	if err := readData(stateURL, &state); err != nil || state.Timestamp != nil || state.Online {
		t.Fatal("restart reused stale state", state, err)
	}
	if err := readData(baseURL+"/api/v1/devices/car-001/telemetry", &history); err != nil || len(history.Items) < 5 {
		t.Fatal("history lost on restart")
	}
	publish(t, pub, "status", testutil.Status("car-001", time.Now(), true))
	eventually(t, "online before Broker loss", func() bool { return readData(stateURL, &state) == nil && state.Online })
	pub.Disconnect(100)
	broker.stop()
	eventually(t, "offline on Broker loss", func() bool { return readData(stateURL, &state) == nil && !state.Online })
	broker = startBroker()
	pub = publisher(t, brokerPort)
	publish(t, pub, "status", testutil.Status("car-001", time.Now(), true))
	eventually(t, "automatic resubscription after Broker restart", func() bool { return readData(stateURL, &state) == nil && state.Online })
	latest = time.Now().UTC()
	publish(t, pub, "telemetry", testutil.Telemetry("car-001", latest, 2))
	eventually(t, "telemetry after Broker restart", func() bool {
		return readData(stateURL, &state) == nil && state.Timestamp != nil && state.Timestamp.Equal(latest)
	})
	bad := testutil.Telemetry("car-001", time.Now().UTC(), 3)
	bad.SchemaVersion = 99
	publish(t, pub, "telemetry", bad)
	fault := testutil.Telemetry("car-001", time.Now().UTC(), 4)
	fault.Temperature.Status = "timeout"
	publish(t, pub, "telemetry", fault)
	eventually(t, "fault without process exit", func() bool {
		return readData(stateURL, &state) == nil && state.Temperature == nil && state.Freshness["temperature"] == "fault"
	})
	t.Logf("verified Mosquitto→actual server→HTTP/2 WS, %d persisted snapshots, will, seq reset, expiry, platform and Broker restart", len(history.Items))
}
