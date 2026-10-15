package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"piguard/go-backend/internal/auth"
	"piguard/go-backend/internal/model"
	"piguard/go-backend/internal/protocol"
	"piguard/go-backend/internal/repo"
	"piguard/go-backend/internal/service"
	"piguard/go-backend/internal/simulator"
	"piguard/go-backend/internal/testutil"
)

type acceptanceFixture struct {
	*frameFixture
	access    *auth.Manager
	events    service.EventService
	commands  *service.CommandManager
	publisher *simulatedCommandPublisher
}

func newAcceptanceFixture(t *testing.T) *acceptanceFixture {
	t.Helper()
	f := &acceptanceFixture{frameFixture: newFrameFixture(t)}
	f.server.Close()
	var err error
	f.access, err = auth.New(auth.Config{PlatformUsername: "platform", PlatformPassword: "test-platform", DevicePasswords: map[string]string{"car-001": "test-device", "car-002": "test-device-2"}, SessionTTLMinutes: 1, AllowedOrigins: []string{"http://127.0.0.1:5173"}}, "127.0.0.1:8080")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(f.access.Close)
	monitor := service.NewMonitor(f.devices, repo.NewTelemetryRepository(f.g), f.hub)
	if err := monitor.Ingest(context.Background(), "car/car-001/status", testutil.JSON(testutil.Status("car-001", time.Now().UTC(), true))); err != nil {
		t.Fatal(err)
	}
	executor, err := simulator.NewCommandExecutor("success")
	if err != nil {
		t.Fatal(err)
	}
	f.publisher = &simulatedCommandPublisher{executor: executor}
	f.publisher.available.Store(true)
	f.commands = service.NewCommandService(repo.NewCommandRepository(f.g), f.devices, f.publisher, monitor.Online, f.hub, f.frames)
	f.publisher.manager = f.commands
	if err := f.commands.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(f.commands.Close)
	f.events = service.NewEventService(repo.NewEventRepository(f.g), f.devices, f.hub, f.frames)
	f.frames.SetEventNotifier(f.events.FrameSaved)
	sql, _ := f.g.DB()
	f.engine = NewEngine(Dependencies{Auth: f.access, Ping: sql.PingContext, Devices: service.NewDeviceService(f.devices, repo.NewConfigRepository(f.g), monitor), Telemetry: service.NewTelemetryService(repo.NewTelemetryRepository(f.g), f.devices), Events: f.events, Commands: f.commands, Configs: service.NewConfigService(repo.NewConfigRepository(f.g), f.publisher, f.devices), Frames: f.frames, Hub: f.hub})
	f.server = httptest.NewServer(f.engine)
	snapshots, err := simulator.NewSnapshotClient(f.server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if err := snapshots.SetCredentials("car-001", simulator.Credentials{Username: "car-001", Password: "test-device"}, false); err != nil {
		t.Fatal(err)
	}
	executor.SetCameraCapture(snapshots.Capture)
	return f
}

func (f *acceptanceFixture) request(t *testing.T, method, path, body, user, password string, headers http.Header, cookies ...*http.Cookie) (int, http.Header, []byte) {
	t.Helper()
	r, err := http.NewRequest(method, f.server.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
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
	for _, cookie := range cookies {
		r.AddCookie(cookie)
	}
	response, err := f.server.Client().Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response.StatusCode, response.Header, raw
}

func TestAuthenticatedRoutesRejectWithoutSideEffects(t *testing.T) {
	f := newAcceptanceFixture(t)
	type route struct{ method, path, body string }
	routes := []route{
		{"GET", "/api/v1/devices", ""}, {"GET", "/api/v1/devices/car-001/state", ""},
		{"GET", "/api/v1/devices/car-001/telemetry", ""}, {"GET", "/api/v1/devices/car-001/events", ""},
		{"GET", "/api/v1/devices/car-001/config", ""}, {"PUT", "/api/v1/devices/car-001/config", `{"expected_version":1,"rules":{"obstacle":{"warning_distance_m":20}}}`},
		{"POST", "/api/v1/devices/car-001/commands", `{"type":"scenario.stop","params":{}}`},
		{"GET", "/api/v1/commands/missing", ""}, {"GET", "/api/v1/devices/car-001/frame", ""},
		{"GET", "/api/v1/snapshots/missing/content", ""}, {"GET", "/api/v1/ws", ""},
		{"POST", "/api/v1/devices/car-002/frames", "invalid"},
		{"POST", "/api/v1/auth/session", ""}, {"GET", "/api/v1/auth/session", ""}, {"DELETE", "/api/v1/auth/session", ""},
	}
	for _, route := range routes {
		for _, identity := range []struct {
			user, password string
			want           int
		}{{"", "", 401}, {"platform", "wrong", 401}, {"car-001", "test-device", 403}} {
			status, headers, raw := f.request(t, route.method, route.path, route.body, identity.user, identity.password, nil)
			if status != identity.want || headers.Get("Cache-Control") != "no-store" {
				t.Fatalf("%s %s: HTTP %d", route.method, route.path, status)
			}
			var env Response
			if json.Unmarshal(raw, &env) != nil || env.Code != map[int]int{401: 40101, 403: 40301}[status] {
				t.Fatal("incorrect auth error")
			}
		}
	}
	for _, table := range []any{&model.Command{}, &model.Snapshot{}} {
		var count int64
		if err := f.g.Model(table).Count(&count).Error; err != nil || count != 0 {
			t.Fatal("rejected request wrote business records", err)
		}
	}
	var cfg model.DeviceConfig
	if err := f.g.Where("device_id = ?", "car-001").First(&cfg).Error; err != nil || cfg.DesiredVersion != 1 {
		t.Fatal("rejected config changed version", err)
	}
	if f.publisher.executor.Executions() != 0 {
		t.Fatal("rejected command executed")
	}
	entries, err := os.ReadDir(f.dir)
	if err != nil || len(entries) != 0 {
		t.Fatal("rejected upload wrote files", err)
	}
	ct, body := frameBody("own-frame", "snapshot", time.Now().UTC().Format(time.RFC3339Nano), jpegFixture(t, 12))
	status, _, raw := f.request(t, "POST", "/api/v1/devices/car-001/frames", string(body), "car-001", "test-device", http.Header{"Content-Type": {ct}})
	if status != 201 {
		t.Fatal("own-device authenticated upload failed", status, string(raw))
	}
	status, _, _ = f.request(t, "GET", "/health", "", "", "", nil)
	if status != 200 {
		t.Fatal("health requires identity")
	}
}

func TestBrowserSessionCORSCSRFAndWebsocketRevocation(t *testing.T) {
	f := newAcceptanceFixture(t)
	origin := "http://127.0.0.1:5173"
	status, headers, raw := f.request(t, "POST", "/api/v1/auth/session", "", "platform", "test-platform", http.Header{"Origin": {origin}})
	if status != 201 || headers.Get("Access-Control-Allow-Origin") != origin || headers.Get("Access-Control-Allow-Credentials") != "true" {
		t.Fatal("login/CORS failed", status)
	}
	var login struct {
		Data struct {
			CSRF    string    `json:"csrf_token"`
			Expires time.Time `json:"expires_at"`
		}
	}
	if err := json.Unmarshal(raw, &login); err != nil || login.Data.CSRF == "" || login.Data.Expires.IsZero() {
		t.Fatal("login has no session fields", err)
	}
	cookies := (&http.Response{Header: headers}).Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode {
		t.Fatal("bad session cookie")
	}
	status, _, _ = f.request(t, "GET", "/api/v1/devices", "", "platform", "wrong", nil, cookies...)
	if status != 401 {
		t.Fatal("invalid Basic fell back to cookie")
	}
	for _, token := range []string{"", "wrong"} {
		status, _, _ = f.request(t, "POST", "/api/v1/devices/car-001/commands", `{"type":"scenario.stop","params":{}}`, "", "", http.Header{"Origin": {origin}, "X-CSRF-Token": {token}}, cookies...)
		if status != 403 {
			t.Fatal("cookie write lacks CSRF protection")
		}
	}
	status, _, _ = f.request(t, "POST", "/api/v1/devices/car-001/commands", `{"type":"scenario.stop","params":{}}`, "", "", http.Header{"Origin": {origin}, "X-CSRF-Token": {login.Data.CSRF}}, cookies...)
	if status != 202 {
		t.Fatal("valid cookie command rejected", status)
	}
	wsHeaders := http.Header{"Origin": {origin}, "Cookie": {cookies[0].Name + "=" + cookies[0].Value}}
	ws, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(f.server.URL, "http")+"/api/v1/ws", wsHeaders)
	if err != nil {
		t.Fatal("cookie WS rejected", err)
	}
	defer ws.Close()
	status, _, _ = f.request(t, "DELETE", "/api/v1/auth/session", "", "", "", http.Header{"Origin": {origin}, "X-CSRF-Token": {login.Data.CSRF}}, cookies...)
	if status != 204 {
		t.Fatal("logout failed")
	}
	ws.SetReadDeadline(time.Now().Add(2 * time.Second))
	for {
		if _, _, err := ws.ReadMessage(); err != nil {
			if e, ok := err.(interface{ Timeout() bool }); ok && e.Timeout() {
				t.Fatal("logout did not close WS")
			}
			break
		}
	}
	status, _, _ = f.request(t, "GET", "/api/v1/auth/session", "", "", "", nil, cookies...)
	if status != 401 {
		t.Fatal("logout cookie still authorized")
	}
	status, _, _ = f.request(t, "POST", "/api/v1/devices/car-001/commands", `{}`, "", "", http.Header{"Origin": {"http://evil.example"}})
	if status != 403 {
		t.Fatal("origin did not precede authentication")
	}
	status, headers, _ = f.request(t, "OPTIONS", "/api/v1/devices/car-001/commands", "", "", "", http.Header{"Origin": {origin}, "Access-Control-Request-Method": {"POST"}, "Access-Control-Request-Headers": {"authorization,content-type,x-csrf-token"}})
	if status != 204 || headers.Get("Access-Control-Allow-Origin") != origin {
		t.Fatal("authenticated preflight failed")
	}
	for _, header := range []http.Header{
		{"Origin": {origin}, "Access-Control-Request-Method": {"PATCH"}},
		{"Origin": {origin}, "Access-Control-Request-Method": {"POST"}, "Access-Control-Request-Headers": {"x-untrusted"}},
		{"Origin": {"null"}, "Access-Control-Request-Method": {"POST"}},
	} {
		status, _, _ = f.request(t, "OPTIONS", "/api/v1/devices", "", "", "", header)
		if status != 403 {
			t.Fatal("invalid preflight accepted")
		}
	}
}

func TestCameraSnapshotAuthenticatedUploadAckAndTwoWS(t *testing.T) {
	f := newAcceptanceFixture(t)
	clients := make([]*websocket.Conn, 2)
	for i := range clients {
		req := httptest.NewRequest("GET", f.server.URL, nil)
		req.SetBasicAuth("platform", "test-platform")
		ws, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(f.server.URL, "http")+"/api/v1/ws", req.Header)
		if err != nil {
			t.Fatal(err)
		}
		clients[i] = ws
		defer ws.Close()
	}
	status, _, raw := f.request(t, "POST", "/api/v1/devices/car-001/commands", `{"type":"camera.snapshot","params":{}}`, "platform", "test-platform", nil)
	if status != 202 {
		t.Fatal("camera not accepted", status)
	}
	var accepted struct {
		Data struct {
			ID string `json:"command_id"`
		}
	}
	if err := json.Unmarshal(raw, &accepted); err != nil {
		t.Fatal(err)
	}
	var view protocol.CommandView
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		status, _, raw = f.request(t, "GET", "/api/v1/commands/"+accepted.Data.ID, "", "platform", "test-platform", nil)
		var env struct{ Data protocol.CommandView }
		if status != 200 || json.Unmarshal(raw, &env) != nil {
			t.Fatal("command query failed")
		}
		view = env.Data
		if view.Status == "success" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if view.Status != "success" || !equalJSON(t, view.Params, json.RawMessage(`{"quality":80}`)) {
		t.Fatal("camera did not finish with normalized params", view.Status)
	}
	var result struct {
		ID  string `json:"snapshot_id"`
		URL string `json:"url"`
	}
	if err := json.Unmarshal(view.Result, &result); err != nil || result.ID == "" || result.URL != protocol.SnapshotURL(result.ID) {
		t.Fatal("invalid camera result", err)
	}
	status, headers, jpeg := f.request(t, "GET", result.URL, "", "platform", "test-platform", nil)
	if status != 200 || headers.Get("Content-Type") != "image/jpeg" || len(jpeg) < 100 {
		t.Fatal("camera content unavailable")
	}
	for _, ws := range clients {
		seenFrame, seenCommand := false, false
		ws.SetReadDeadline(time.Now().Add(3 * time.Second))
		for !seenFrame || !seenCommand {
			var event struct {
				Type string
				Data json.RawMessage
			}
			if err := ws.ReadJSON(&event); err != nil {
				t.Fatal(err)
			}
			if event.Type == "frame_update" {
				var frame protocol.FramePush
				json.Unmarshal(event.Data, &frame)
				seenFrame = frame.SnapshotID == result.ID
			}
			if event.Type == "command_update" {
				var cmd protocol.CommandView
				json.Unmarshal(event.Data, &cmd)
				if cmd.CommandID == view.CommandID && cmd.Status == "success" {
					if !equalJSON(t, cmd.Result, view.Result) {
						t.Fatal("HTTP/WS result mismatch")
					}
					seenCommand = true
				}
			}
		}
	}
	row, err := f.snapshots.Find(context.Background(), result.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(f.dir, row.Path)); err != nil {
		t.Fatal(err)
	}
	status, _, _ = f.request(t, "GET", result.URL, "", "platform", "test-platform", nil)
	if status != 404 {
		t.Fatal("missing image not reflected")
	}
	if saved, err := f.commands.Get(context.Background(), view.CommandID); err != nil || saved.Status != "success" {
		t.Fatal("missing image rewrote command fact", err)
	}
}

func TestEventImageBothOrdersAndIdempotentNotifications(t *testing.T) {
	f := newFrameFixture(t)
	events := service.NewEventService(repo.NewEventRepository(f.g), f.devices, f.hub, f.frames)
	f.frames.SetEventNotifier(events.FrameSaved)
	ctx := context.Background()
	for _, first := range []bool{true, false} {
		name := "event-first"
		if first {
			name = "image-first"
		}
		t.Run(name, func(t *testing.T) {
			sub := f.hub.Subscribe()
			defer f.hub.Unsubscribe(sub)
			at := time.Now().UTC().Format(time.RFC3339Nano)
			raw := alarmRaw("evt-"+name, "car-001", "obstacle_warning", "started", "warning", at, `{}`, map[string]any{"frame_id": name})
			if first {
				f.upload(t, "car-001", name, "event", at, jpegFixture(t, 1), 201)
			}
			if err := events.Ingest(ctx, "car/car-001/events", raw); err != nil {
				t.Fatal(err)
			}
			items, err := events.List(ctx, "car-001", repo.EventQuery{})
			if err != nil {
				t.Fatal(err)
			}
			want := "pending"
			if first {
				want = "available"
			}
			var initial protocol.EventItem
			for _, item := range items {
				if item.EventID == "evt-"+name {
					initial = item
				}
			}
			if initial.SnapshotState != want {
				t.Fatal("wrong initial image state", initial)
			}
			if !first {
				f.upload(t, "car-001", name, "event", at, jpegFixture(t, 1), 201)
			}
			items, err = events.List(ctx, "car-001", repo.EventQuery{})
			if err != nil {
				t.Fatal(err)
			}
			var linked protocol.EventItem
			for _, item := range items {
				if item.EventID == "evt-"+name {
					linked = item
				}
			}
			if linked.SnapshotState != "available" || linked.SnapshotID == nil || !linked.SnapshotAvailable || linked.SnapshotURL == nil {
				t.Fatal("image did not link", linked)
			}
			kinds := map[string]int{}
			for len(sub.Messages) > 0 {
				var e struct{ Type string }
				json.Unmarshal(<-sub.Messages, &e)
				kinds[e.Type]++
			}
			updates := 1
			if first {
				updates = 0
			}
			if kinds["event"] != 1 || kinds["event_image_update"] != updates || kinds["frame_update"] != 1 {
				t.Fatal("incorrect publication sequence", kinds)
			}
			if err := events.Ingest(ctx, "car/car-001/events", raw); err != nil {
				t.Fatal(err)
			}
			f.upload(t, "car-001", name, "event", at, jpegFixture(t, 1), 200)
			if len(sub.Messages) != 0 {
				t.Fatal("idempotent retry published again")
			}
			row, err := f.snapshots.Find(ctx, *linked.SnapshotID)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(filepath.Join(f.dir, row.Path)); err != nil {
				t.Fatal(err)
			}
			image, err := f.frames.ResolveEventImage(ctx, "car-001", "", name)
			if err != nil || image.SnapshotState != "missing" || image.SnapshotID != nil {
				t.Fatal("missing file exposed as available", err)
			}
		})
	}
}
