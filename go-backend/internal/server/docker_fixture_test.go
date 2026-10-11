//go:build integration

package server_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"
	"github.com/gorilla/websocket"
)

type dockerFixture struct {
	browser                                                                   *httptest.Server
	t                                                                         *testing.T
	root, project, envFile, override, baseURL, broker, simulator              string
	platformHTTP, deviceHTTP, platformMQTT, deviceMQTT, healthMQTT, otherMQTT string
	client                                                                    *http.Client
}

func newDockerFixture(t *testing.T) *dockerFixture {
	t.Helper()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	f := &dockerFixture{t: t, root: root, project: "piguard-acceptance-" + strings.ToLower(rand.Text()[:10]), envFile: filepath.Join(dir, "acceptance.env"), override: filepath.Join(dir, "compose.yaml"), client: &http.Client{Timeout: 8 * time.Second}}
	f.platformHTTP = rand.Text() + "$!#= space"
	f.deviceHTTP = rand.Text()
	f.platformMQTT = rand.Text()
	f.deviceMQTT = rand.Text()
	f.healthMQTT = rand.Text()
	f.otherMQTT = rand.Text()
	f.browser = httptest.NewServer(http.HandlerFunc(f.browserPage))
	t.Cleanup(f.browser.Close)
	devices, _ := json.Marshal(map[string]string{"car-001": f.deviceHTTP, "car-002": f.deviceHTTP})
	origins, _ := json.Marshal([]string{"http://127.0.0.1:5173", f.browser.URL})
	env := fmt.Sprintf("PIGUARD_HTTP_PLATFORM_USERNAME=platform\nPIGUARD_HTTP_PLATFORM_PASSWORD='%s'\nPIGUARD_HTTP_SEED_DEVICE_PASSWORD='%s'\nPIGUARD_HTTP_SESSION_TTL_MINUTES=1\nPIGUARD_HTTP_ALLOWED_ORIGINS='%s'\nPIGUARD_SEED_DEVICE_ID=car-001\nPIGUARD_MQTT_USERNAME=platform\nPIGUARD_MQTT_PASSWORD='%s'\nPIGUARD_DEVICE_MQTT_PASSWORD='%s'\nPIGUARD_BROKER_HEALTH_USERNAME=broker-health\nPIGUARD_BROKER_HEALTH_PASSWORD='%s'\nPIGUARD_TEST_OTHER_MQTT_PASSWORD='%s'\nPIGUARD_TEST_HTTP_DEVICES='%s'\n", f.platformHTTP, f.deviceHTTP, origins, f.platformMQTT, f.deviceMQTT, f.healthMQTT, f.otherMQTT, devices)
	if err := os.WriteFile(f.envFile, []byte(env), 0600); err != nil {
		t.Fatal(err)
	}
	initPath := filepath.Join(dir, "init.sh")
	init := `#!/bin/sh
set -eu
/piguard/init-broker-auth.sh
mosquitto_passwd -b /mosquitto/auth/passwords car-002 "$PIGUARD_TEST_OTHER_MQTT_PASSWORD" >/dev/null
{
  printf '\nuser car-002\ntopic read car/car-002/commands\ntopic read car/car-002/config\n'
  for topic in telemetry events status command-acks config-acks; do printf 'topic write car/car-002/%s\n' "$topic"; done
} >> /mosquitto/auth/acl
chown mosquitto:mosquitto /mosquitto/auth/passwords /mosquitto/auth/acl
chmod 640 /mosquitto/auth/passwords /mosquitto/auth/acl
`
	if err := os.WriteFile(initPath, []byte(init), 0600); err != nil {
		t.Fatal(err)
	}
	override := fmt.Sprintf("services:\n  broker-init:\n    entrypoint: [/bin/sh, /piguard/test-init.sh]\n    environment:\n      PIGUARD_TEST_OTHER_MQTT_PASSWORD: ${PIGUARD_TEST_OTHER_MQTT_PASSWORD}\n    volumes:\n      - %s\n  mosquitto:\n    ports: !override\n      - '127.0.0.1:%d:1883'\n  backend:\n    ports: !override\n      - '127.0.0.1:%d:8080'\n    environment:\n      PIGUARD_HTTP_DEVICE_PASSWORDS: ${PIGUARD_TEST_HTTP_DEVICES}\n", strconv.Quote(filepath.ToSlash(initPath)+":/piguard/test-init.sh:ro"), freePort(t), freePort(t))
	if err := os.WriteFile(f.override, []byte(override), 0600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if t.Failed() {
			t.Log(f.redact(f.composeOutput("logs", "--no-color", "--tail", "100")))
		}
		f.compose("down", "--volumes", "--remove-orphans", "--timeout", "10")
	})
	f.compose("config", "--quiet")
	f.compose("up", "-d", "--build", "--wait", "--wait-timeout", "180")
	f.baseURL = "http://" + strings.TrimSpace(f.compose("port", "backend", "8080"))
	f.broker = "tcp://" + strings.TrimSpace(f.compose("port", "mosquitto", "1883"))
	f.simulator = filepath.Join(dir, "simulator")
	if runtime.GOOS == "windows" {
		f.simulator += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", f.simulator, "./cmd/simulator")
	cmd.Dir = root
	if raw, err := cmd.CombinedOutput(); err != nil {
		t.Fatal("build simulator:", string(raw), err)
	}
	// Register the second device using the actual seed path and the same isolated volume.
	seed := f.project + "-seed"
	f.compose("run", "-d", "--no-deps", "--name", seed, "-e", "PIGUARD_SEED_DEVICE_ID=car-002", "-e", "PIGUARD_MQTT_CLIENT_ID=acceptance-seed", "backend")
	f.wait("second device registration", func() bool {
		status, _ := f.request("GET", "/api/v1/devices/car-002", nil, "platform", f.platformHTTP, nil)
		return status == 200
	})
	f.docker("stop", seed)
	f.docker("rm", seed)
	t.Log("isolated authenticated Compose ready; two devices registered")
	return f
}

func (f *dockerFixture) redact(raw string) string {
	for _, secret := range []string{f.platformHTTP, f.deviceHTTP, f.platformMQTT, f.deviceMQTT, f.healthMQTT, f.otherMQTT} {
		if secret != "" {
			raw = strings.ReplaceAll(raw, secret, "[redacted]")
		}
	}
	return raw
}
func (f *dockerFixture) composeArgs(args ...string) []string {
	return append([]string{"compose", "--project-name", f.project, "--env-file", f.envFile, "-f", filepath.Join(f.root, "docker-compose.yml"), "-f", f.override}, args...)
}
func (f *dockerFixture) composeOutput(args ...string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", f.composeArgs(args...)...)
	cmd.Dir = f.root
	raw, _ := cmd.CombinedOutput()
	return string(raw)
}
func (f *dockerFixture) compose(args ...string) string { return f.docker(f.composeArgs(args...)...) }
func (f *dockerFixture) docker(args ...string) string {
	f.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Dir = f.root
	raw, err := cmd.CombinedOutput()
	if err != nil {
		f.t.Fatalf("Docker action %s: %v\n%s", args[0], err, f.redact(string(raw)))
	}
	return string(raw)
}
func (f *dockerFixture) wait(what string, fn func() bool) {
	f.t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	f.t.Fatal("timed out:", what)
}
func (f *dockerFixture) request(method, path string, body []byte, user, password string, headers http.Header) (int, []byte) {
	r, err := http.NewRequest(method, f.baseURL+path, bytes.NewReader(body))
	if err != nil {
		f.t.Fatal(err)
	}
	if headers != nil {
		r.Header = headers.Clone()
	}
	if r.Header.Get("Content-Type") == "" {
		r.Header.Set("Content-Type", "application/json")
	}
	if user != "" {
		r.SetBasicAuth(user, password)
	}
	resp, err := f.client.Do(r)
	if err != nil {
		return 0, nil
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		f.t.Fatal(err)
	}
	return resp.StatusCode, raw
}
func (f *dockerFixture) data(method, path string, body []byte, want int, dst any) {
	f.t.Helper()
	status, raw := f.request(method, path, body, "platform", f.platformHTTP, nil)
	if status != want {
		f.t.Fatalf("%s %s: HTTP %d; %s", method, path, status, f.redact(string(raw)))
	}
	if dst != nil {
		var env struct {
			Code int
			Data json.RawMessage
		}
		if err := json.Unmarshal(raw, &env); err != nil || env.Code != 0 {
			f.t.Fatal("invalid response envelope", err)
		}
		if err := json.Unmarshal(env.Data, dst); err != nil {
			f.t.Fatal(err)
		}
	}
}
func (f *dockerFixture) mqtt(user, password string, handler paho.MessageHandler) paho.Client {
	f.t.Helper()
	opts := paho.NewClientOptions().AddBroker(f.broker).SetClientID("acceptance-" + rand.Text()).SetUsername(user).SetPassword(password).SetConnectTimeout(3 * time.Second).SetDefaultPublishHandler(handler).SetAutoReconnect(true)
	c := paho.NewClient(opts)
	token := c.Connect()
	if !token.WaitTimeout(5*time.Second) || token.Error() != nil {
		f.t.Fatal("MQTT identity connection failed")
	}
	f.t.Cleanup(func() { c.Disconnect(100) })
	return c
}
func (f *dockerFixture) publish(c paho.Client, topic string, data any, retained bool) {
	f.t.Helper()
	raw, err := json.Marshal(data)
	if err != nil {
		f.t.Fatal(err)
	}
	token := c.Publish(topic, 1, retained, raw)
	if !token.WaitTimeout(5*time.Second) || token.Error() != nil {
		f.t.Fatal("MQTT publish failed", topic)
	}
}
func (f *dockerFixture) startSimulator(device, mode string, extra ...string) *process {
	password := f.deviceMQTT
	if device == "car-002" {
		password = f.otherMQTT
	}
	args := append([]string{"-device", device, "-duration", "40m", "-broker", f.broker, "-api", f.baseURL, "-ack-mode", mode}, extra...)
	cmd := exec.Command(f.simulator, args...)
	cmd.Env = append(os.Environ(), "PIGUARD_SIM_MQTT_USERNAME="+device, "PIGUARD_SIM_MQTT_PASSWORD="+password, "PIGUARD_SIM_HTTP_USERNAME="+device, "PIGUARD_SIM_HTTP_PASSWORD="+f.deviceHTTP)
	p := &process{cmd: cmd, done: make(chan error, 1), log: &safeLog{}}
	cmd.Stdout = p.log
	cmd.Stderr = p.log
	if err := cmd.Start(); err != nil {
		f.t.Fatal(err)
	}
	go func() { p.done <- cmd.Wait() }()
	f.t.Cleanup(func() {
		p.stop()
		if f.t.Failed() {
			f.t.Log(f.redact(p.log.String()))
		}
	})
	f.wait(device+" online", func() bool {
		status, raw := f.request("GET", "/api/v1/devices/"+device+"/state", nil, "platform", f.platformHTTP, nil)
		return status == 200 && bytes.Contains(raw, []byte(`"online":true`))
	})
	return p
}
func (f *dockerFixture) socket(headers http.Header) *websocket.Conn {
	f.t.Helper()
	if headers == nil {
		r, _ := http.NewRequest("GET", f.baseURL, nil)
		r.SetBasicAuth("platform", f.platformHTTP)
		headers = r.Header
	}
	c, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(f.baseURL, "http")+"/api/v1/ws", headers)
	if err != nil {
		f.t.Fatal(err)
	}
	f.t.Cleanup(func() { c.Close() })
	return c
}
