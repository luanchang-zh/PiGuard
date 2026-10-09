package protocol

import (
	"bytes"
	"context"
	"image/jpeg"
	"io"
	"mime"
	"mime/multipart"
	"piguard/go-backend/internal/apperr"
	"regexp"
	"time"
)

const MaxJPEGBytes = 2 << 20
const MaxFrameBodyBytes = MaxJPEGBytes + (64 << 10)
const MaxJPEGEdge = 4096
const MaxJPEGPixels = 16777216

var frameIdentity = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
var frameTime = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(\.[0-9]+)?(Z|[+-](0[0-9]|1[0-9]|2[0-3]):[0-5][0-9])$`)

type FrameUpload struct {
	FrameID    string
	CapturedAt time.Time
	Type       string
	JPEG       []byte
}
type FrameResult struct {
	SnapshotID string    `json:"snapshot_id"`
	DeviceID   string    `json:"device_id"`
	Type       string    `json:"type"`
	CapturedAt time.Time `json:"captured_at"`
}
type FrameView struct {
	SnapshotID string    `json:"snapshot_id"`
	Type       string    `json:"type"`
	CapturedAt time.Time `json:"captured_at"`
	URL        string    `json:"url"`
}
type FramePush struct {
	FrameView
	DeviceID string `json:"device_id"`
}

// ContextReader bounds each read and observes cancellation during decoding/copying.
type ContextReader struct {
	Context context.Context
	Reader  io.Reader
}

func (r ContextReader) Read(p []byte) (int, error) {
	if err := r.Context.Err(); err != nil {
		return 0, err
	}
	if len(p) > 32<<10 {
		p = p[:32<<10]
	}
	return r.Reader.Read(p)
}
func frameInvalid(field string) error { return &apperr.InvalidParams{Field: field} }
func ParseFrame(ctx context.Context, contentType string, raw []byte) (*FrameUpload, error) {
	if len(raw) > MaxFrameBodyBytes {
		return nil, &apperr.FrameTooLarge{Field: "body"}
	}
	media, params, err := mime.ParseMediaType(contentType)
	if err != nil || media != "multipart/form-data" || params["boundary"] == "" {
		return nil, frameInvalid("body")
	}
	r := multipart.NewReader(ContextReader{ctx, bytes.NewReader(raw)}, params["boundary"])
	fields := map[string][]byte{}
	for {
		part, err := r.NextRawPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, frameInvalid("body")
		}
		media, disposition, err := mime.ParseMediaType(part.Header.Get("Content-Disposition"))
		name := disposition["name"]
		if err != nil || media != "form-data" || name == "" {
			return nil, frameInvalid("body")
		}
		switch name {
		case "file", "frame_id", "captured_at", "type":
		default:
			return nil, frameInvalid(name)
		}
		if _, exists := fields[name]; exists {
			return nil, frameInvalid(name)
		}
		if _, file := disposition["filename"]; file && name != "file" {
			return nil, frameInvalid(name)
		}
		value, err := io.ReadAll(part)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, frameInvalid(name)
		}
		fields[name] = value
	}
	for _, name := range []string{"file", "frame_id", "captured_at", "type"} {
		if _, exists := fields[name]; !exists {
			return nil, frameInvalid(name)
		}
	}
	if !frameTime.Match(fields["captured_at"]) {
		return nil, frameInvalid("captured_at")
	}
	at, err := time.Parse(time.RFC3339Nano, string(fields["captured_at"]))
	if err != nil {
		return nil, frameInvalid("captured_at")
	}
	return &FrameUpload{FrameID: string(fields["frame_id"]), CapturedAt: at.UTC(), Type: string(fields["type"]), JPEG: fields["file"]}, nil
}
func (u FrameUpload) Validate(ctx context.Context) error {
	if !frameIdentity.MatchString(u.FrameID) {
		return frameInvalid("frame_id")
	}
	if u.CapturedAt.UTC().Year() < 0 || u.CapturedAt.UTC().Year() > 9999 {
		return frameInvalid("captured_at")
	}
	switch u.Type {
	case "preview", "snapshot", "event":
	default:
		return frameInvalid("type")
	}
	if len(u.JPEG) > MaxJPEGBytes {
		return &apperr.FrameTooLarge{Field: "file"}
	}
	if len(u.JPEG) == 0 {
		return frameInvalid("file")
	}
	config, err := jpeg.DecodeConfig(ContextReader{ctx, bytes.NewReader(u.JPEG)})
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		return frameInvalid("file")
	}
	if config.Width > MaxJPEGEdge || config.Height > MaxJPEGEdge || int64(config.Width)*int64(config.Height) > MaxJPEGPixels {
		return &apperr.FrameTooLarge{Field: "file"}
	}
	_, err = jpeg.Decode(ContextReader{ctx, bytes.NewReader(u.JPEG)})
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		return frameInvalid("file")
	}
	return nil
}
