package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"gorm.io/gorm"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"path/filepath"
	"piguard/go-backend/internal/db"
	"piguard/go-backend/internal/model"
	"piguard/go-backend/internal/protocol"
	"piguard/go-backend/internal/realtime"
	"piguard/go-backend/internal/repo"
	"piguard/go-backend/internal/service"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

type frameFixture struct {
	path, dir string
	g         *gorm.DB
	devices   repo.DeviceRepository
	snapshots repo.SnapshotRepository
	hub       *realtime.Hub
	frames    service.FrameService
	engine    *gin.Engine
	server    *httptest.Server
}

func newFrameFixture(t *testing.T) *frameFixture {
	t.Helper()
	root := t.TempDir()
	f := &frameFixture{path: filepath.Join(root, "frames.db"), dir: filepath.Join(root, "images")}
	if err := os.MkdirAll(f.dir, 0700); err != nil {
		t.Fatal(err)
	}
	f.open(t)
	t.Cleanup(f.close)
	return f
}
func (f *frameFixture) open(t *testing.T) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	var err error
	f.g, err = db.Open(f.path)
	if err != nil {
		t.Fatal(err)
	}
	f.devices = repo.NewDeviceRepository(f.g)
	f.snapshots = repo.NewSnapshotRepository(f.g)
	f.hub = realtime.NewHub(128)
	devices := service.NewDeviceService(f.devices, repo.NewConfigRepository(f.g))
	for _, id := range []string{"car-001", "car-002"} {
		if _, err := devices.Seed(context.Background(), service.SeedInput{DeviceID: id, Name: id, Mode: "mock", SoftwareVersion: "0.1.0"}); err != nil {
			t.Fatal(err)
		}
	}
	f.frames = service.NewFrameService(f.snapshots, f.devices, f.dir, f.hub)
	f.engine = NewEngine(Dependencies{Devices: devices, Frames: f.frames, Hub: f.hub})
	f.server = httptest.NewServer(f.engine)
}
func (f *frameFixture) close() {
	if f.hub != nil {
		f.hub.Close()
		f.hub = nil
	}
	if f.server != nil {
		f.server.Close()
		f.server = nil
	}
	if f.g != nil {
		s, _ := f.g.DB()
		_ = s.Close()
		f.g = nil
	}
}
func jpegFixture(t *testing.T, shade uint8) []byte {
	t.Helper()
	var b bytes.Buffer
	im := image.NewRGBA(image.Rect(0, 0, 16, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			im.Set(x, y, color.RGBA{shade, uint8(x * 8), uint8(y * 8), 255})
		}
	}
	if err := jpeg.Encode(&b, im, nil); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func frameBody(id, typ, at string, raw []byte) (string, []byte) {
	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	_ = w.WriteField("frame_id", id)
	_ = w.WriteField("captured_at", at)
	_ = w.WriteField("type", typ)
	h := textproto.MIMEHeader{"Content-Disposition": {`form-data; name="file"; filename="../../not-a-jpeg.png"`}, "Content-Type": {"application/octet-stream"}}
	p, _ := w.CreatePart(h)
	_, _ = p.Write(raw)
	_ = w.Close()
	return w.FormDataContentType(), b.Bytes()
}
func frameRequest(client *http.Client, url, id, typ, at string, raw []byte) (int, protocol.FrameResult, error) {
	ct, body := frameBody(id, typ, at, raw)
	req, _ := http.NewRequest("POST", url, bytes.NewReader(body))
	req.Header.Set("Content-Type", ct)
	r, err := client.Do(req)
	if err != nil {
		return 0, protocol.FrameResult{}, err
	}
	defer r.Body.Close()
	var env struct {
		Code    int                  `json:"code"`
		Data    protocol.FrameResult `json:"data"`
		Message string               `json:"message"`
	}
	err = json.NewDecoder(r.Body).Decode(&env)
	if err != nil {
		return r.StatusCode, env.Data, err
	}
	if r.StatusCode == 200 || r.StatusCode == 201 {
		if env.Code != 0 || env.Data.SnapshotID == "" {
			return r.StatusCode, env.Data, fmt.Errorf("bad upload: %+v", env)
		}
	}
	if expected, ok := map[int]int{400: 40001, 404: 40401, 409: 40903, 413: 40001, 500: 50001}[r.StatusCode]; ok && env.Code != expected {
		return r.StatusCode, env.Data, fmt.Errorf("wrong error code: got %d want %d", env.Code, expected)
	}
	return r.StatusCode, env.Data, nil
}
func (f *frameFixture) upload(t *testing.T, device, id, typ, at string, raw []byte, want int) protocol.FrameResult {
	t.Helper()
	status, data, err := frameRequest(f.server.Client(), f.server.URL+"/api/v1/devices/"+device+"/frames", id, typ, at, raw)
	if err != nil || status != want {
		t.Fatalf("upload %s: %d %+v %v", id, status, data, err)
	}
	return data
}
func (f *frameFixture) latest(t *testing.T, device string) protocol.FrameView {
	t.Helper()
	r, err := http.Get(f.server.URL + "/api/v1/devices/" + device + "/frame")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	var env struct {
		Code int                `json:"code"`
		Data protocol.FrameView `json:"data"`
	}
	err = json.NewDecoder(r.Body).Decode(&env)
	if err != nil || r.StatusCode != 200 || env.Code != 0 {
		t.Fatal(r.StatusCode, env, err)
	}
	return env.Data
}
func (f *frameFixture) content(t *testing.T, url string, expected []byte) {
	t.Helper()
	r, err := http.Get(f.server.URL + url)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	raw, err := io.ReadAll(r.Body)
	if err != nil || r.StatusCode != 200 || r.Header.Get("Content-Type") != "image/jpeg" || !bytes.Equal(raw, expected) {
		t.Fatal("content", r.StatusCode, r.Header, err)
	}
}
func (f *frameFixture) expectError(t *testing.T, path string, status, code int) {
	t.Helper()
	r, err := http.Get(f.server.URL + path)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	raw, _ := io.ReadAll(r.Body)
	var env Response
	if err = json.Unmarshal(raw, &env); err != nil || r.StatusCode != status || env.Code != code || bytes.Contains(raw, []byte(f.dir)) {
		t.Fatal(r.StatusCode, string(raw), err)
	}
}
func frameSocket(t *testing.T, f *frameFixture) *websocket.Conn {
	t.Helper()
	c, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(f.server.URL, "http")+"/api/v1/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}
func framePush(t *testing.T, c *websocket.Conn) protocol.FramePush {
	t.Helper()
	_ = c.SetReadDeadline(time.Now().Add(3 * time.Second))
	var env struct {
		Type      string             `json:"type"`
		Timestamp time.Time          `json:"timestamp"`
		Data      protocol.FramePush `json:"data"`
	}
	if err := c.ReadJSON(&env); err != nil || env.Type != "frame_update" || env.Timestamp.IsZero() || env.Data.URL == "" {
		t.Fatal(env, err)
	}
	return env.Data
}
func TestFramesHTTPContentTwoWSAndRestart(t *testing.T) {
	f := newFrameFixture(t)
	ctx := context.Background()
	raw := jpegFixture(t, 150)
	before, err := f.devices.Find(ctx, "car-001")
	if err != nil {
		t.Fatal(err)
	}
	var configBefore model.DeviceConfig
	if err = f.g.First(&configBefore, "device_id = ?", "car-001").Error; err != nil {
		t.Fatal(err)
	}
	f.expectError(t, "/api/v1/devices/car-001/frame", 404, 40403)
	f.expectError(t, "/api/v1/devices/missing/frame", 404, 40401)
	f.expectError(t, "/api/v1/snapshots/missing/content", 404, 40403)
	a, b := frameSocket(t, f), frameSocket(t, f)
	snapshot := f.upload(t, "car-001", "same-id", "snapshot", "2026-10-09T20:00:00.123456789+08:00", raw, 201)
	if snapshot.DeviceID != "car-001" || snapshot.Type != "snapshot" || snapshot.CapturedAt.Location() != time.UTC || snapshot.CapturedAt.Hour() != 12 || snapshot.CapturedAt.Nanosecond() != 123456789 {
		t.Fatal(snapshot)
	}
	f.expectError(t, "/api/v1/devices/car-001/frame", 404, 40403)
	pa, pb := framePush(t, a), framePush(t, b)
	if !reflect.DeepEqual(pa, pb) || pa.SnapshotID != snapshot.SnapshotID {
		t.Fatal(pa, pb)
	}
	f.content(t, pa.URL, raw)
	event := f.upload(t, "car-001", "event-1", "event", "2026-10-09T12:00:01Z", raw, 201)
	pa, pb = framePush(t, a), framePush(t, b)
	if !reflect.DeepEqual(pa, pb) || pa.SnapshotID != event.SnapshotID {
		t.Fatal(pa, pb)
	}
	f.content(t, pa.URL, raw)
	preview := f.upload(t, "car-001", "preview-1", "preview", "2026-10-09T12:00:02.1Z", raw, 201)
	pa, pb = framePush(t, a), framePush(t, b)
	if !reflect.DeepEqual(pa, pb) || pa.SnapshotID != preview.SnapshotID {
		t.Fatal(pa, pb)
	}
	if current := f.latest(t, "car-001"); current.SnapshotID != preview.SnapshotID {
		t.Fatal(current)
	}
	// Duplicate with a different textual timezone is the same normalized metadata.
	retry := f.upload(t, "car-001", "same-id", "snapshot", "2026-10-09T12:00:00.123456789Z", raw, 200)
	if retry.SnapshotID != snapshot.SnapshotID {
		t.Fatal(retry)
	}
	for _, change := range []struct {
		typ, at string
		raw     []byte
	}{
		{"event", "2026-10-09T12:00:00.123456789Z", raw},
		{"snapshot", "2026-10-09T12:00:00.123456788Z", raw},
		{"snapshot", "2026-10-09T12:00:00.123456789Z", jpegFixture(t, 42)},
	} {
		f.upload(t, "car-001", "same-id", change.typ, change.at, change.raw, 409)
	}
	f.upload(t, "missing", "x", "preview", "2026-10-09T12:00:00Z", raw, 404)
	old := f.upload(t, "car-001", "old", "preview", "2026-10-09T12:00:01.999999999Z", raw, 201)
	f.content(t, "/api/v1/snapshots/"+old.SnapshotID+"/content", raw)
	tie := f.upload(t, "car-001", "tie", "preview", "2026-10-09T12:00:02.1Z", raw, 201)
	pa, pb = framePush(t, a), framePush(t, b)
	if pa.SnapshotID != tie.SnapshotID || !reflect.DeepEqual(pa, pb) {
		t.Fatal("unexpected duplicate, old frame, or ordering", pa, pb)
	}
	if f.latest(t, "car-001").SnapshotID != tie.SnapshotID {
		t.Fatal("tie did not advance")
	}
	other := f.upload(t, "car-002", "same-id", "snapshot", "2026-10-09T12:00:00.123456789Z", raw, 201)
	if other.SnapshotID == snapshot.SnapshotID {
		t.Fatal("cross-device collision")
	}
	_ = framePush(t, a)
	_ = framePush(t, b)
	after, _ := f.devices.Find(ctx, "car-001")
	if !reflect.DeepEqual(before, after) {
		t.Fatal("upload changed device facts", before, after)
	}
	var configAfter model.DeviceConfig
	_ = f.g.First(&configAfter, "device_id = ?", "car-001").Error
	if !reflect.DeepEqual(configBefore, configAfter) {
		t.Fatal("upload changed config")
	}
	for _, c := range []*websocket.Conn{a, b} {
		_ = c.SetReadDeadline(time.Now().Add(80 * time.Millisecond))
		if _, _, err := c.ReadMessage(); err == nil {
			t.Fatal("extra push")
		}
		_ = c.Close()
	}
	// An unreferenced file remains outside the API, and does not replace the latest.
	if err := os.WriteFile(filepath.Join(f.dir, "orphan.jpg"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	f.close()
	f.open(t)
	if f.latest(t, "car-001").SnapshotID != tie.SnapshotID {
		t.Fatal("lost latest after restart")
	}
	f.content(t, pa.URL, raw)
	retry = f.upload(t, "car-001", "same-id", "snapshot", "2026-10-09T12:00:00.123456789Z", raw, 200)
	if retry.SnapshotID != snapshot.SnapshotID {
		t.Fatal("lost identity")
	}
	f.expectError(t, "/api/v1/snapshots/orphan/content", 404, 40403)
}
func TestFramesHTTPConcurrentIdentityAndPreviewOrder(t *testing.T) {
	f := newFrameFixture(t)
	raw := jpegFixture(t, 10)
	ctx := context.Background()
	sub := f.hub.Subscribe()
	defer f.hub.Unsubscribe(sub)
	var wg sync.WaitGroup
	type reply struct {
		status int
		data   protocol.FrameResult
		err    error
	}
	responses := make(chan reply, 24)
	for range 24 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			status, data, err := frameRequest(f.server.Client(), f.server.URL+"/api/v1/devices/car-001/frames", "concurrent", "preview", "2026-10-09T12:00:00Z", raw)
			responses <- reply{status, data, err}
		}()
	}
	wg.Wait()
	close(responses)
	created := 0
	id := ""
	for r := range responses {
		if r.err != nil || (r.status != 200 && r.status != 201) {
			t.Fatal(r)
		}
		if r.status == 201 {
			created++
		}
		if id == "" {
			id = r.data.SnapshotID
		}
		if id != r.data.SnapshotID {
			t.Fatal("duplicate identity")
		}
	}
	if created != 1 || len(sub.Messages) != 1 {
		t.Fatal(created, len(sub.Messages))
	}
	<-sub.Messages
	var count int64
	_ = f.g.Model(&model.Snapshot{}).Count(&count).Error
	entries, _ := os.ReadDir(f.dir)
	if count != 1 || len(entries) != 1 {
		t.Fatal(count, len(entries))
	}
	// Race different contents under one key: first wins; losing requests conflict.
	responses = make(chan reply, 12)
	for i := range 12 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			body := raw
			if i%2 == 1 {
				body = append(bytes.Clone(raw), byte(i))
			}
			status, data, err := frameRequest(f.server.Client(), f.server.URL+"/api/v1/devices/car-001/frames", "mixed", "snapshot", "2026-10-09T12:01:00Z", body)
			responses <- reply{status, data, err}
		}(i)
	}
	wg.Wait()
	close(responses)
	created = 0
	conflicts := 0
	for r := range responses {
		if r.err != nil {
			t.Fatal(r)
		}
		switch r.status {
		case 201:
			created++
		case 409:
			conflicts++
		case 200:
		default:
			t.Fatal(r)
		}
	}
	if created != 1 || conflicts == 0 || len(sub.Messages) != 1 {
		t.Fatal(created, conflicts, len(sub.Messages))
	}
	<-sub.Messages
	// Preview commits and notifications are monotonic even when arrival order races.
	responses = make(chan reply, 24)
	for i := range 24 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			at := time.Date(2026, 10, 9, 12, 2, 0, i, time.UTC)
			status, data, err := frameRequest(f.server.Client(), f.server.URL+"/api/v1/devices/car-001/frames", fmt.Sprintf("order-%d", i), "preview", at.Format(time.RFC3339Nano), raw)
			responses <- reply{status, data, err}
		}(i)
	}
	wg.Wait()
	close(responses)
	for r := range responses {
		if r.err != nil || r.status != 201 {
			t.Fatal(r)
		}
	}
	var last time.Time
	lastID := ""
	for len(sub.Messages) > 0 {
		var env struct {
			Data protocol.FramePush `json:"data"`
		}
		if err := json.Unmarshal(<-sub.Messages, &env); err != nil {
			t.Fatal(err)
		}
		if env.Data.CapturedAt.Before(last) {
			t.Fatal("WS preview regressed")
		}
		last = env.Data.CapturedAt
		lastID = env.Data.SnapshotID
		f.content(t, env.Data.URL, raw)
	}
	latest := f.latest(t, "car-001")
	if latest.CapturedAt.Nanosecond() != 23 || latest.SnapshotID != lastID {
		t.Fatal(latest, lastID)
	}
	row, err := f.snapshots.FindLatest(ctx, "car-001")
	if err != nil || row.SnapshotID != latest.SnapshotID {
		t.Fatal(row, err)
	}
	entries, _ = os.ReadDir(f.dir)
	_ = f.g.Model(&model.Snapshot{}).Count(&count).Error
	if count != 26 || len(entries) != 26 {
		t.Fatal(count, len(entries))
	}
}
func TestFramesHTTPRejectionAndStorageErrors(t *testing.T) {
	f := newFrameFixture(t)
	raw := jpegFixture(t, 50)
	sub := f.hub.Subscribe()
	defer f.hub.Unsubscribe(sub)
	for _, test := range []struct {
		name, field, ct string
		body            []byte
		status          int
		chunked         bool
	}{
		{"body-limit", "body", "multipart/form-data; boundary=x", make([]byte, protocol.MaxFrameBodyBytes+1), 413, false},
		{"chunked-limit", "body", "multipart/form-data; boundary=x", make([]byte, protocol.MaxFrameBodyBytes+1), 413, true},
		{"not-multipart", "body", "application/json", []byte(`{}`), 400, false},
		{"broken-multipart", "body", "multipart/form-data; boundary=x", []byte("--x\r\nbroken"), 400, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			req, _ := http.NewRequest("POST", f.server.URL+"/api/v1/devices/car-001/frames", bytes.NewReader(test.body))
			req.Header.Set("Content-Type", test.ct)
			if test.chunked {
				req.ContentLength = -1
			}
			r, err := f.server.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer r.Body.Close()
			var env struct {
				Code  int `json:"code"`
				Error struct {
					Field string `json:"field"`
				} `json:"error"`
			}
			if err = json.NewDecoder(r.Body).Decode(&env); err != nil || r.StatusCode != test.status || env.Code != 40001 || env.Error.Field != test.field {
				t.Fatal(r.StatusCode, env, err)
			}
		})
	}
	for _, test := range []struct {
		id, typ, at string
		body        []byte
		want        int
	}{
		{"bad", "preview", "2026-10-09T12:00:00Z", []byte("not jpeg"), 400},
		{"bad", "preview", "2026-10-09T12:00:00Z", raw[:len(raw)-2], 400},
		{"../bad", "preview", "2026-10-09T12:00:00Z", raw, 400},
		{"bad", "video", "2026-10-09T12:00:00Z", raw, 400},
		{"bad", "preview", "bad", raw, 400},
		{"big", "preview", "2026-10-09T12:00:00Z", make([]byte, protocol.MaxJPEGBytes+1), 413},
	} {
		f.upload(t, "car-001", test.id, test.typ, test.at, test.body, test.want)
	}
	var count int64
	_ = f.g.Model(&model.Snapshot{}).Count(&count).Error
	entries, _ := os.ReadDir(f.dir)
	if count != 0 || len(entries) != 0 || len(sub.Messages) != 0 {
		t.Fatal("reject persisted/pushed", count, len(entries), len(sub.Messages))
	}
	// Actual SQLite write failure rolls back and removes this upload's file.
	if err := f.g.Exec("CREATE TRIGGER reject_snapshots BEFORE INSERT ON snapshots BEGIN SELECT RAISE(ABORT, 'injected write failure'); END").Error; err != nil {
		t.Fatal(err)
	}
	f.upload(t, "car-001", "db-fail", "preview", "2026-10-09T12:00:00Z", raw, 500)
	_ = f.g.Exec("DROP TRIGGER reject_snapshots").Error
	entries, _ = os.ReadDir(f.dir)
	if len(entries) != 0 || len(sub.Messages) != 0 {
		t.Fatal("failure residue", len(entries), len(sub.Messages))
	}
	if err := os.Rename(f.dir, f.dir+"-saved"); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(f.dir, []byte("directory unavailable"), 0600)
	f.upload(t, "car-001", "disk-fail", "preview", "2026-10-09T12:00:00Z", raw, 500)
	_ = os.Remove(f.dir)
	_ = os.Rename(f.dir+"-saved", f.dir)
	created := f.upload(t, "car-001", "read", "preview", "2026-10-09T12:00:00Z", raw, 201)
	row, err := f.snapshots.Find(context.Background(), created.SnapshotID)
	if err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/snapshots/" + created.SnapshotID + "/content"
	if err = os.Remove(filepath.Join(f.dir, row.Path)); err != nil {
		t.Fatal(err)
	}
	f.expectError(t, path, 404, 40403)
	if err = os.Mkdir(filepath.Join(f.dir, row.Path), 0700); err != nil {
		t.Fatal(err)
	}
	f.expectError(t, path, 500, 50001)
	_ = os.Remove(filepath.Join(f.dir, row.Path))
	secret := filepath.Join(t.TempDir(), "secret.txt")
	_ = os.WriteFile(secret, []byte("private-content"), 0600)
	if err = os.Symlink(secret, filepath.Join(f.dir, row.Path)); err != nil {
		t.Fatal(err)
	}
	f.expectError(t, path, 500, 50001)
	if err = f.g.Model(&model.Snapshot{}).Where("snapshot_id = ?", created.SnapshotID).Update("path", secret).Error; err != nil {
		t.Fatal(err)
	}
	f.expectError(t, path, 500, 50001)
	sql, _ := f.g.DB()
	_ = sql.Close()
	f.expectError(t, path, 500, 50001)
}

func TestFramesHTTPResourceBoundaries(t *testing.T) {
	f := newFrameFixture(t)
	small := jpegFixture(t, 80)
	exact := append(bytes.Clone(small), make([]byte, protocol.MaxJPEGBytes-len(small))...)
	result := f.upload(t, "car-001", "byte-boundary", "event", "2026-10-09T12:00:00Z", exact, 201)
	f.content(t, "/api/v1/snapshots/"+result.SnapshotID+"/content", exact)
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, image.NewGray(image.Rect(0, 0, 4096, 4096)), nil); err != nil {
		t.Fatal(err)
	}
	result = f.upload(t, "car-001", "pixel-boundary", "snapshot", "2026-10-09T12:00:00Z", encoded.Bytes(), 201)
	f.content(t, "/api/v1/snapshots/"+result.SnapshotID+"/content", encoded.Bytes())
	tooWide := bytes.Clone(small)
	for i := 2; i+8 < len(tooWide); i++ {
		if tooWide[i] == 255 && tooWide[i+1] == 0xc0 {
			tooWide[i+7] = 0x10
			tooWide[i+8] = 0x01
			break
		}
	}
	f.upload(t, "car-001", "over-dimension", "preview", "2026-10-09T12:00:00Z", tooWide, 413)
	ct, body := frameBody("body-boundary", "preview", "2026-10-09T12:00:00Z", small)
	body = append(body, make([]byte, protocol.MaxFrameBodyBytes-len(body))...)
	req, _ := http.NewRequest("POST", f.server.URL+"/api/v1/devices/car-001/frames", bytes.NewReader(body))
	req.Header.Set("Content-Type", ct)
	response, err := f.server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 201 {
		raw, _ := io.ReadAll(response.Body)
		t.Fatal(response.StatusCode, string(raw))
	}
	if view := f.latest(t, "car-001"); view.Type != "preview" {
		t.Fatal(view)
	}
}
