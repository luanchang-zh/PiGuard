package simulator

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"piguard/go-backend/internal/protocol"
)

func TestSnapshotUploadRetryIdentityAndAuthentication(t *testing.T) {
	for _, status := range []int{401, 403, 400, 408, 429, 500} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var attempts atomic.Int32
			var original []byte
			var mu sync.Mutex
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				attempt := attempts.Add(1)
				user, password, ok := r.BasicAuth()
				if !ok || user != "car-001" || password != "test-device" {
					t.Error("upload credentials missing")
				}
				raw, err := io.ReadAll(r.Body)
				if err != nil {
					t.Error(err)
				}
				mu.Lock()
				if attempt == 1 {
					original = raw
				} else if !bytes.Equal(raw, original) {
					t.Error("retry changed upload identity or bytes")
				}
				mu.Unlock()
				w.WriteHeader(status)
			}))
			defer srv.Close()
			client, err := NewSnapshotClient(srv.URL)
			if err != nil {
				t.Fatal(err)
			}
			if err := client.SetCredentials("car-001", Credentials{Username: "car-001", Password: "test-device"}, false); err != nil {
				t.Fatal(err)
			}
			_, err = client.UploadEvent(context.Background(), "car-001", "evt-retry", time.Now().UTC())
			if err == nil {
				t.Fatal("upload rejection returned success")
			}
			want := int32(3)
			if status == 401 || status == 403 || status == 400 {
				want = 1
			}
			if attempts.Load() != want {
				t.Fatal("incorrect retry count", attempts.Load(), want)
			}
		})
	}
}

func TestSnapshotUploadLostResponseAndCancellation(t *testing.T) {
	var attempts atomic.Int32
	var original []byte
	var mu sync.Mutex
	at := time.Now().UTC()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		if attempts.Add(1) == 1 {
			mu.Lock()
			original = raw
			mu.Unlock()
			conn, _, err := w.(http.Hijacker).Hijack()
			if err != nil {
				t.Error(err)
				return
			}
			conn.Close()
			return
		}
		mu.Lock()
		if !bytes.Equal(raw, original) {
			t.Error("lost-response retry changed upload")
		}
		mu.Unlock()
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": protocol.FrameResult{SnapshotID: "snap-recovered", DeviceID: "car-001", Type: "event", CapturedAt: at}})
	}))
	defer srv.Close()
	client, err := NewSnapshotClient(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.UploadEvent(context.Background(), "car-001", "evt-lost", at)
	if err != nil || result.SnapshotID != "snap-recovered" || attempts.Load() != 2 {
		t.Fatal("lost response was not recovered", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.UploadEvent(ctx, "car-001", "evt-canceled", at); err == nil || attempts.Load() != 2 {
		t.Fatal("canceled upload performed HTTP work")
	}
}
