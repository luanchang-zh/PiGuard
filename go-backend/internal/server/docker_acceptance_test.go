//go:build integration

package server_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"
	"github.com/gorilla/websocket"
	"piguard/go-backend/internal/protocol"
	"piguard/go-backend/internal/simulator"
)

func dockerWSReader(t *testing.T, c *websocket.Conn) <-chan wsEvent {
	t.Helper()
	ch := make(chan wsEvent, 1024)
	go func() {
		defer close(ch)
		for {
			var e wsEvent
			if c.ReadJSON(&e) != nil {
				return
			}
			select {
			case ch <- e:
			default:
				return
			}
		}
	}()
	return ch
}
func waitDockerWS(t *testing.T, ch <-chan wsEvent, kind, id string) json.RawMessage {
	t.Helper()
	timer := time.NewTimer(20 * time.Second)
	defer timer.Stop()
	for {
		select {
		case e, ok := <-ch:
			if !ok {
				t.Fatal("WS closed before expected event")
			}
			if e.Type == kind && bytes.Contains(e.Data, []byte(id)) {
				return e.Data
			}
		case <-timer.C:
			t.Fatal("missing WS event", kind, id)
		}
	}
}
func (f *dockerFixture) createCommand(kind string) protocol.CommandView {
	f.t.Helper()
	params := `{}`
	switch kind {
	case "buzzer.test":
		params = `{"duration_ms":100}`
	case "indicator.test":
		params = `{"color":"green","duration_ms":100}`
	case "scenario.start":
		params = `{"scenario":"normal_drive","speed":1}`
	}
	var accepted struct {
		ID     string `json:"command_id"`
		Status string `json:"status"`
	}
	f.data("POST", "/api/v1/devices/car-001/commands", []byte(fmt.Sprintf(`{"type":%q,"params":%s}`, kind, params)), 202, &accepted)
	if accepted.ID == "" || accepted.Status != "pending" {
		f.t.Fatal("command acceptance missing durable identity")
	}
	var view protocol.CommandView
	f.data("GET", "/api/v1/commands/"+accepted.ID, nil, 200, &view)
	return view
}
func (f *dockerFixture) commandStatus(id, want string) protocol.CommandView {
	var view protocol.CommandView
	f.wait("command "+want, func() bool { f.data("GET", "/api/v1/commands/"+id, nil, 200, &view); return view.Status == want })
	return view
}
func TestDockerAcceptance(t *testing.T) {
	f := newDockerFixture(t)
	for _, tc := range []struct {
		user, password string
		want           int
	}{{"", "", 401}, {"platform", "wrong", 401}, {"car-001", f.deviceHTTP, 403}, {"platform", f.platformHTTP, 200}} {
		status, _ := f.request("GET", "/api/v1/devices", nil, tc.user, tc.password, nil)
		if status != tc.want {
			t.Fatal("HTTP role boundary failed", status)
		}
	}
	for _, creds := range [][2]string{{"", ""}, {"car-001", "wrong"}} {
		c := paho.NewClient(paho.NewClientOptions().AddBroker(f.broker).SetClientID("denied-client").SetUsername(creds[0]).SetPassword(creds[1]).SetConnectTimeout(2 * time.Second))
		token := c.Connect()
		if !token.WaitTimeout(4*time.Second) || token.Error() == nil {
			c.Disconnect(100)
			t.Fatal("Broker allowed invalid credentials")
		}
	}
	t.Log("HTTP roles, special-character password and Broker authentication verified")
	first := dockerWSReader(t, f.socket(nil))
	second := dockerWSReader(t, f.socket(nil))
	device := f.startSimulator("car-001", "success", "-event-image")
	other := f.startSimulator("car-002", "success", "-config-ack-mode", "failed")
	var state protocol.State
	f.wait("telemetry", func() bool {
		f.data("GET", "/api/v1/devices/car-001/state", nil, 200, &state)
		return state.Distance != nil && *state.Distance == 18 && state.Temperature != nil && state.Timestamp != nil
	})
	for _, ch := range []<-chan wsEvent{first, second} {
		waitDockerWS(t, ch, "telemetry", "car-001")
	}
	var events struct {
		Items []protocol.EventItem `json:"items"`
	}
	f.wait("linked event JPEG", func() bool {
		f.data("GET", "/api/v1/devices/car-001/events", nil, 200, &events)
		return len(events.Items) == 1 && events.Items[0].SnapshotState == "available"
	})
	imageURL := *events.Items[0].SnapshotURL
	status, image := f.request("GET", imageURL, nil, "platform", f.platformHTTP, nil)
	if status != 200 || len(image) < 100 {
		t.Fatal("event image cannot be read")
	}
	var previous protocol.CommandView
	for _, kind := range []string{"buzzer.test", "indicator.test", "scenario.start", "scenario.stop", "camera.snapshot"} {
		view := f.createCommand(kind)
		view = f.commandStatus(view.CommandID, "success")
		if view.AckAt == nil || view.ExecutedAt == nil {
			t.Fatal("successful command lacks acknowledgement")
		}
		for _, ch := range []<-chan wsEvent{first, second} {
			for {
				raw := waitDockerWS(t, ch, "command_update", view.CommandID)
				var pushed protocol.CommandView
				if json.Unmarshal(raw, &pushed) != nil {
					t.Fatal("invalid command WS")
				}
				if pushed.Status == "success" {
					if !bytes.Equal(pushed.Result, view.Result) {
						t.Fatal("HTTP and WS result differ")
					}
					break
				}
			}
		}
		if kind == "camera.snapshot" {
			var result struct {
				ID  string `json:"snapshot_id"`
				URL string `json:"url"`
			}
			json.Unmarshal(view.Result, &result)
			status, raw := f.request("GET", result.URL, nil, "platform", f.platformHTTP, nil)
			if result.ID == "" || status != 200 || len(raw) < 100 {
				t.Fatal("snapshot upload/Ack/content failed")
			}
		}
		previous = view
	}
	var cfg protocol.ConfigView
	f.wait("initial retained config Ack", func() bool {
		f.data("GET", "/api/v1/devices/car-001/config", nil, 200, &cfg)
		return cfg.ReportedVersion == cfg.DesiredVersion && cfg.Status == "success"
	})
	version := cfg.DesiredVersion
	f.data("PUT", "/api/v1/devices/car-001/config", []byte(fmt.Sprintf(`{"expected_version":%d,"rules":{"obstacle":{"warning_distance_m":20}}}`, version)), 202, nil)
	f.wait("updated config Ack", func() bool {
		f.data("GET", "/api/v1/devices/car-001/config", nil, 200, &cfg)
		return cfg.DesiredVersion == version+1 && cfg.ReportedVersion == version+1 && cfg.Status == "success"
	})
	f.data("PUT", "/api/v1/devices/car-001/config", []byte(fmt.Sprintf(`{"expected_version":%d,"rules":{"obstacle":{"warning_distance_m":21}}}`, version)), 409, nil)
	f.wait("device config failure", func() bool {
		var failed protocol.ConfigView
		f.data("GET", "/api/v1/devices/car-002/config", nil, 200, &failed)
		return failed.Status == "failed" && failed.ReportedVersion == 0
	})
	t.Log("real MQTT telemetry/2 WS, duplicate event/image, action/camera and config success/failure/conflict verified")
	device.stop()
	f.wait("device offline", func() bool { f.data("GET", "/api/v1/devices/car-001/state", nil, 200, &state); return !state.Online })
	f.data("POST", "/api/v1/devices/car-001/commands", []byte(`{"type":"scenario.stop","params":{}}`), 503, nil)
	failed := f.startSimulator("car-001", "failed")
	view := f.createCommand("scenario.stop")
	f.commandStatus(view.CommandID, "failed")
	failed.stop()
	none := f.startSimulator("car-001", "none")
	view = f.createCommand("scenario.stop")
	f.commandStatus(view.CommandID, "timeout")
	none.stop()
	late := f.startSimulator("car-001", "success", "-ack-delay", "12s")
	view = f.createCommand("scenario.stop")
	f.commandStatus(view.CommandID, "timeout")
	time.Sleep(3 * time.Second)
	f.commandStatus(view.CommandID, "timeout")
	if err := late.cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	f.wait("offline last will", func() bool { f.data("GET", "/api/v1/devices/car-001/state", nil, 200, &state); return !state.Online })
	other.stop()
	t.Log("device offline rejection, failed/no/late Ack, and abrupt-exit last will verified")
	f.testACL()
	f.testBrowserSession()
	f.data("PUT", "/api/v1/devices/car-001/config", []byte(fmt.Sprintf(`{"expected_version":%d,"rules":{"obstacle":{"warning_distance_m":22}}}`, version+1)), 202, nil)
	f.data("GET", "/api/v1/devices/car-001/config", nil, 200, &cfg)
	if cfg.DesiredVersion != version+2 || cfg.ReportedVersion != version+1 || cfg.Status != "pending" {
		t.Fatal("offline configuration did not remain pending")
	}
	var history struct {
		Items []json.RawMessage `json:"items"`
	}
	f.data("GET", "/api/v1/devices/car-001/telemetry?limit=1000", nil, 200, &history)
	if len(history.Items) == 0 {
		t.Fatal("network telemetry was not persisted")
	}
	historyCount := len(history.Items)
	// Retained configuration and durable facts must survive both kinds of restart.
	f.compose("stop", "mosquitto")
	f.wait("Broker disconnect", func() bool { f.data("GET", "/api/v1/devices/car-001/state", nil, 200, &state); return !state.Online })
	f.compose("start", "mosquitto")
	f.wait("Broker healthy", func() bool {
		return strings.Contains(f.compose("ps", "--format", "json", "mosquitto"), `"Health":"healthy"`)
	})
	retained := make(chan bool, 4)
	reader := f.mqtt("car-001", f.deviceMQTT, func(_ paho.Client, m paho.Message) {
		var c protocol.ConfigMessage
		if json.Unmarshal(m.Payload(), &c) == nil && c.ConfigVersion == version+2 {
			retained <- m.Retained()
		}
	})
	token := reader.Subscribe("car/car-001/config", 1, nil)
	if !token.WaitTimeout(5*time.Second) || token.Error() != nil {
		t.Fatal("restored config subscription failed")
	}
	select {
	case isRetained := <-retained:
		if !isRetained {
			t.Fatal("restored config is not retained")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("latest configuration not restored after Broker restart")
	}
	f.compose("restart", "backend")
	f.wait("backend health", func() bool { status, _ := f.request("GET", "/health", nil, "", "", nil); return status == 200 })
	var restored protocol.CommandView
	f.data("GET", "/api/v1/commands/"+previous.CommandID, nil, 200, &restored)
	if restored.Status != "success" || !bytes.Equal(restored.Result, previous.Result) {
		t.Fatal("restart lost command fact")
	}
	f.data("GET", "/api/v1/devices/car-001/config", nil, 200, &cfg)
	if cfg.ReportedVersion != version+1 || cfg.DesiredVersion != version+2 || cfg.Status != "pending" {
		t.Fatal("restart lost offline config facts")
	}
	f.data("GET", "/api/v1/devices/car-001/telemetry?limit=1000", nil, 200, &history)
	if len(history.Items) < historyCount {
		t.Fatal("restart lost telemetry history")
	}
	status, restoredImage := f.request("GET", imageURL, nil, "platform", f.platformHTTP, nil)
	if status != 200 || !bytes.Equal(image, restoredImage) {
		t.Fatal("restart lost image bytes")
	}
	device = f.startSimulator("car-001", "success", "-event-image")
	f.wait("post-restart telemetry", func() bool {
		f.data("GET", "/api/v1/devices/car-001/state", nil, 200, &state)
		return state.Online && state.Timestamp != nil
	})
	f.wait("offline configuration eventually confirmed", func() bool {
		f.data("GET", "/api/v1/devices/car-001/config", nil, 200, &cfg)
		return cfg.ReportedVersion == version+2 && cfg.Status == "success"
	})
	t.Log("Broker/backend restart, SQLite/JPEG persistence, restored subscriptions and config verified")
	if duration := os.Getenv("PIGUARD_SOAK_DURATION"); duration != "" {
		d, err := time.ParseDuration(duration)
		if err != nil || d <= 0 {
			t.Fatal("invalid soak duration")
		}
		f.soak(d)
	}
	device.stop()
	logs := f.compose("logs", "--no-color")
	for _, secret := range []string{f.platformHTTP, f.deviceHTTP, f.platformMQTT, f.deviceMQTT, f.healthMQTT, f.otherMQTT} {
		if strings.Contains(logs, secret) {
			t.Fatal("runtime logs disclosed a credential")
		}
	}
	t.Log("runtime logs contain none of the generated credential values")
}

func (f *dockerFixture) testACL() {
	t := f.t
	seen := make(chan string, 128)
	platform := f.mqtt("platform", f.platformMQTT, func(_ paho.Client, m paho.Message) { seen <- string(m.Payload()) })
	token := platform.Subscribe("car/+/events", 1, nil)
	if !token.WaitTimeout(5*time.Second) || token.Error() != nil {
		t.Fatal("platform event subscription failed")
	}
	attackerSeen := make(chan string, 128)
	attacker := f.mqtt("car-001", f.deviceMQTT, func(_ paho.Client, m paho.Message) { attackerSeen <- string(m.Payload()) })
	for _, topic := range []string{"car/car-001/commands", "car/car-002/commands"} {
		token = attacker.Subscribe(topic, 1, nil)
		if !token.WaitTimeout(5 * time.Second) {
			t.Fatal("ACL subscribe timed out")
		}
	}
	other := f.mqtt("car-002", f.otherMQTT, nil)
	marker := "acl-event-" + fmt.Sprint(time.Now().UnixNano())
	event := simulator.DemoEvent("car-002", marker, time.Now().UTC())
	f.publish(attacker, "car/car-002/events", event, false)
	select {
	case <-seen:
		t.Fatal("cross-device publication delivered")
	case <-time.After(300 * time.Millisecond):
	}
	f.publish(other, "car/car-002/events", event, false)
	select {
	case raw := <-seen:
		if !strings.Contains(raw, marker) {
			t.Fatal("unexpected ACL control message")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("allowed device publication did not deliver")
	}
	for _, device := range []string{"car-002", "car-001"} {
		f.publish(platform, "car/"+device+"/commands", map[string]string{"acl_probe": device}, false)
	}
	select {
	case raw := <-attackerSeen:
		if !strings.Contains(raw, "car-001") {
			t.Fatal("device received another device command")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("own command permission missing")
	}
	select {
	case raw := <-attackerSeen:
		t.Fatal("unexpected cross-device message", raw)
	case <-time.After(300 * time.Millisecond):
	}
	healthSeen := make(chan string, 16)
	health := f.mqtt("broker-health", f.healthMQTT, func(_ paho.Client, m paho.Message) { healthSeen <- m.Topic() })
	for _, topic := range []string{"$SYS/broker/uptime", "car/+/events"} {
		token = health.Subscribe(topic, 1, nil)
		if !token.WaitTimeout(5 * time.Second) {
			t.Fatal("health subscribe timed out")
		}
	}
	event.EventID += "-health"
	f.publish(other, "car/car-002/events", event, false)
	timer := time.NewTimer(500 * time.Millisecond)
	defer timer.Stop()
	for {
		select {
		case topic := <-healthSeen:
			if strings.HasPrefix(topic, "car/") {
				t.Fatal("health account received business data")
			}
		case <-timer.C:
			t.Log("actual multi-device publish/subscribe ACL and health business isolation verified")
			return
		}
	}
}

func (f *dockerFixture) testBrowserSession() {
	t := f.t
	origin := "http://127.0.0.1:5173"
	r, _ := http.NewRequest("POST", f.baseURL+"/api/v1/auth/session", nil)
	r.SetBasicAuth("platform", f.platformHTTP)
	r.Header.Set("Origin", origin)
	response, err := f.client.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || response.StatusCode != 201 || response.Header.Get("Access-Control-Allow-Origin") != origin {
		t.Fatal("session/CORS failed")
	}
	var session struct {
		Data struct {
			CSRF string `json:"csrf_token"`
		}
	}
	json.Unmarshal(raw, &session)
	cookies := response.Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly {
		t.Fatal("missing browser cookie")
	}
	headers := http.Header{"Origin": {origin}, "Cookie": {cookies[0].Name + "=" + cookies[0].Value}}
	status, _ := f.request("GET", "/api/v1/devices", nil, "", "", headers)
	if status != 200 {
		t.Fatal("cookie query rejected")
	}
	status, _ = f.request("POST", "/api/v1/devices/car-001/commands", []byte(`{"type":"scenario.stop","params":{}}`), "", "", headers)
	if status != 403 {
		t.Fatal("missing CSRF accepted")
	}
	ws := f.socket(headers)
	headers.Set("X-CSRF-Token", session.Data.CSRF)
	status, _ = f.request("DELETE", "/api/v1/auth/session", nil, "", "", headers)
	if status != 204 {
		t.Fatal("logout failed")
	}
	ws.SetReadDeadline(time.Now().Add(3 * time.Second))
	for {
		if _, _, err := ws.ReadMessage(); err != nil {
			if e, ok := err.(interface{ Timeout() bool }); ok && e.Timeout() {
				t.Fatal("logout did not close WS")
			}
			break
		}
	}
	status, _ = f.request("GET", "/api/v1/devices", nil, "", "", headers)
	if status != 401 {
		t.Fatal("revoked cookie accepted")
	}
	t.Log("network Cookie/CORS/CSRF and logout WS lifecycle verified")
}

func (f *dockerFixture) soak(duration time.Duration) {
	t := f.t
	t.Logf("browser test page: %s", f.browser.URL)
	deadline := time.Now().Add(duration)
	nextStats := time.Now()
	iterations := 0
	for time.Now().Before(deadline) {
		view := f.createCommand("scenario.stop")
		f.commandStatus(view.CommandID, "success")
		ws := f.socket(nil)
		ws.Close()
		iterations++
		if iterations%10 == 0 {
			view = f.createCommand("camera.snapshot")
			f.commandStatus(view.CommandID, "success")
		}
		if !time.Now().Before(nextStats) {
			id := strings.TrimSpace(f.compose("ps", "-q", "backend"))
			stats := strings.TrimSpace(f.docker("stats", "--no-stream", "--format", "{{.MemUsage}} | {{.PIDs}}", id))
			fds := strings.TrimSpace(f.compose("exec", "-T", "backend", "sh", "-c", "ls /proc/1/fd | wc -l"))
			t.Logf("soak iterations=%d memory/PIDs=%s open-fds=%s", iterations, stats, fds)
			nextStats = time.Now().Add(time.Minute)
		}
		time.Sleep(2 * time.Second)
	}
	t.Logf("soak complete: %s, %d command and WS reconnect cycles", duration, iterations)
}
