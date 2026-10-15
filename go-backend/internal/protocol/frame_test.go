package protocol

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"mime/multipart"
	"piguard/go-backend/internal/apperr"
	"strings"
	"testing"
	"time"
)

func frameJPEG(t *testing.T) []byte {
	t.Helper()
	var out bytes.Buffer
	im := image.NewRGBA(image.Rect(0, 0, 8, 8))
	im.Set(0, 0, color.RGBA{200, 20, 10, 255})
	if err := jpeg.Encode(&out, im, nil); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}
func dimensionJPEG(t *testing.T, data []byte, width, height int) []byte {
	t.Helper()
	raw := bytes.Clone(data)
	for i := 2; i+8 < len(raw); i++ {
		if raw[i] == 255 && (raw[i+1] == 0xc0 || raw[i+1] == 0xc2) {
			raw[i+5] = byte(height >> 8)
			raw[i+6] = byte(height)
			raw[i+7] = byte(width >> 8)
			raw[i+8] = byte(width)
			return raw
		}
	}
	t.Fatal("no SOF")
	return nil
}
func TestFrameValidation(t *testing.T) {
	good := frameJPEG(t)
	var pngBytes bytes.Buffer
	_ = png.Encode(&pngBytes, image.NewGray(image.Rect(0, 0, 8, 8)))
	base := FrameUpload{FrameID: "frame_1-OK", CapturedAt: time.Date(2026, 10, 9, 10, 0, 0, 123, time.UTC), Type: "preview", JPEG: good}
	for _, test := range []struct {
		name, field string
		large       bool
		mutate      func(*FrameUpload)
	}{
		{"empty-id", "frame_id", false, func(u *FrameUpload) { u.FrameID = "" }},
		{"long-id", "frame_id", false, func(u *FrameUpload) { u.FrameID = strings.Repeat("a", 65) }},
		{"path-id", "frame_id", false, func(u *FrameUpload) { u.FrameID = "../x" }},
		{"unicode-id", "frame_id", false, func(u *FrameUpload) { u.FrameID = "图片" }},
		{"space-id", "frame_id", false, func(u *FrameUpload) { u.FrameID = "a b" }},
		{"type", "type", false, func(u *FrameUpload) { u.Type = "video" }},
		{"empty-file", "file", false, func(u *FrameUpload) { u.JPEG = nil }},
		{"png", "file", false, func(u *FrameUpload) { u.JPEG = pngBytes.Bytes() }},
		{"fake", "file", false, func(u *FrameUpload) { u.JPEG = []byte("not jpeg") }},
		{"truncated", "file", false, func(u *FrameUpload) { u.JPEG = good[:len(good)-2] }},
		{"header-only", "file", false, func(u *FrameUpload) { u.JPEG = good[:len(good)/2] }},
		{"over-bytes", "file", true, func(u *FrameUpload) { u.JPEG = make([]byte, MaxJPEGBytes+1) }},
		{"over-width", "file", true, func(u *FrameUpload) { u.JPEG = dimensionJPEG(t, good, 4097, 1) }},
		{"over-height", "file", true, func(u *FrameUpload) { u.JPEG = dimensionJPEG(t, good, 1, 4097) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			u := base
			test.mutate(&u)
			err := u.Validate(context.Background())
			if test.large {
				var e *apperr.FrameTooLarge
				if !errors.As(err, &e) || e.Field != test.field {
					t.Fatal(err)
				}
			} else {
				var e *apperr.InvalidParams
				if !errors.As(err, &e) || e.Field != test.field {
					t.Fatal(err)
				}
			}
		})
	}
	for _, typ := range []string{"preview", "snapshot", "event"} {
		u := base
		u.Type = typ
		if err := u.Validate(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	u := base
	u.JPEG = append(bytes.Clone(good), make([]byte, MaxJPEGBytes-len(good))...)
	if err := u.Validate(context.Background()); err != nil {
		t.Fatal("exact byte limit", err)
	}
	u = base
	u.FrameID = strings.Repeat("a", 64)
	if err := u.Validate(context.Background()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := base.Validate(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
func TestFrameMultipart(t *testing.T) {
	good := frameJPEG(t)
	fields := []string{"frame_id", "captured_at", "type", "file"}
	values := map[string][]byte{"frame_id": []byte("f-1"), "captured_at": []byte("2026-10-09T20:00:00.123456789+08:00"), "type": []byte("event"), "file": good}
	build := func(names []string, replace map[string][]byte) (string, []byte) {
		var b bytes.Buffer
		w := multipart.NewWriter(&b)
		for _, name := range names {
			v, ok := replace[name]
			if !ok {
				v = values[name]
			}
			if name == "file" {
				p, _ := w.CreateFormFile(name, "../../fake.png")
				_, _ = p.Write(v)
			} else {
				_ = w.WriteField(name, string(v))
			}
		}
		_ = w.Close()
		return w.FormDataContentType(), b.Bytes()
	}
	ct, raw := build(fields, nil)
	u, err := ParseFrame(context.Background(), ct, raw)
	if err != nil || u.CapturedAt.Location() != time.UTC || u.CapturedAt.Nanosecond() != 123456789 || u.CapturedAt.Hour() != 12 {
		t.Fatal(u, err)
	}
	if err = u.Validate(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name    string
		names   []string
		replace map[string][]byte
		field   string
	}{
		{"missing-file", fields[:3], nil, "file"},
		{"duplicate-file", append(append([]string{}, fields...), "file"), nil, "file"},
		{"duplicate-id", append(append([]string{}, fields...), "frame_id"), nil, "frame_id"},
		{"unknown", append(append([]string{}, fields...), "surprise"), nil, "surprise"},
		{"missing-zone", fields, map[string][]byte{"captured_at": []byte("2026-10-09T20:00:00")}, "captured_at"},
		{"invalid-date", fields, map[string][]byte{"captured_at": []byte("2026-02-30T20:00:00Z")}, "captured_at"},
		{"invalid-zone", fields, map[string][]byte{"captured_at": []byte("2026-10-09T20:00:00+24:00")}, "captured_at"},
		{"comma-time", fields, map[string][]byte{"captured_at": []byte("2026-10-09T20:00:00,123Z")}, "captured_at"},
	} {
		t.Run(test.name, func(t *testing.T) {
			ct, raw := build(test.names, test.replace)
			_, err := ParseFrame(context.Background(), ct, raw)
			var e *apperr.InvalidParams
			if !errors.As(err, &e) || e.Field != test.field {
				t.Fatal(err)
			}
		})
	}
	for _, contentType := range []string{"application/json", "multipart/form-data", "multipart/form-data; boundary=wrong"} {
		if _, err := ParseFrame(context.Background(), contentType, raw); err == nil {
			t.Fatal(contentType)
		}
	}
	if _, err := ParseFrame(context.Background(), ct, raw[:len(raw)-20]); err == nil {
		t.Fatal("accepted incomplete multipart")
	}
}
