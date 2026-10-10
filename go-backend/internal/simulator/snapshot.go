package simulator

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"time"

	"piguard/go-backend/internal/protocol"
)

type SnapshotClient struct {
	api         string
	client      *http.Client
	credentials Credentials
}

func NewSnapshotClient(api string) (*SnapshotClient, error) {
	u, err := url.Parse(api)
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return nil, fmt.Errorf("api must be an HTTP(S) base URL without credentials, query or fragment")
	}
	return &SnapshotClient{api: strings.TrimRight(u.String(), "/"), client: &http.Client{Timeout: 6 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

func (s *SnapshotClient) SetCredentials(device string, credentials Credentials, allowEmpty bool) error {
	if err := credentials.Validate(device, allowEmpty); err != nil {
		return err
	}
	s.credentials = credentials
	return nil
}

func (s *SnapshotClient) Capture(ctx context.Context, cmd protocol.Command, at time.Time) (json.RawMessage, error) {
	ctx, cancel := context.WithDeadline(ctx, cmd.ExpiresAt)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	p, err := protocol.ParseSnapshotParams(cmd.Params)
	if err != nil {
		return nil, err
	}
	result, err := s.saveJPEG(ctx, cmd.DeviceID, softwareFrameID("cam-", cmd.CommandID), cmd.CommandID, "snapshot", p.Quality, at)
	if err != nil {
		return nil, err
	}
	return json.Marshal(protocol.SnapshotResult{SnapshotID: result.SnapshotID})
}

func softwareFrameID(prefix, id string) string {
	sum := sha256.Sum256([]byte(id))
	return prefix + hex.EncodeToString(sum[:16])
}

func EventFrameID(eventID string) string {
	return softwareFrameID("evt-", eventID)
}

func (s *SnapshotClient) UploadEvent(ctx context.Context, device, eventID string, at time.Time) (protocol.FrameResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return s.saveJPEG(ctx, device, EventFrameID(eventID), eventID, "event", 80, at)
}

func (s *SnapshotClient) saveJPEG(ctx context.Context, device, frameID, seed, kind string, quality int, at time.Time) (protocol.FrameResult, error) {
	if err := ctx.Err(); err != nil {
		return protocol.FrameResult{}, err
	}
	identity := sha256.Sum256([]byte(seed))
	img := image.NewRGBA(image.Rect(0, 0, 320, 180))
	for y := 0; y < img.Bounds().Dy(); y++ {
		for x := 0; x < img.Bounds().Dx(); x++ {
			img.SetRGBA(x, y, color.RGBA{R: uint8(x) ^ identity[0], G: uint8(y) ^ identity[1], B: identity[2], A: 255})
		}
	}
	var jpegBytes bytes.Buffer
	if err := jpeg.Encode(&jpegBytes, img, &jpeg.Options{Quality: quality}); err != nil {
		return protocol.FrameResult{}, err
	}
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	file, err := w.CreateFormFile("file", "snapshot.jpg")
	if err != nil {
		return protocol.FrameResult{}, err
	}
	if _, err := file.Write(jpegBytes.Bytes()); err != nil {
		return protocol.FrameResult{}, err
	}
	for _, field := range [][2]string{{"frame_id", frameID}, {"captured_at", at.UTC().Format(time.RFC3339Nano)}, {"type", kind}} {
		if err := w.WriteField(field[0], field[1]); err != nil {
			return protocol.FrameResult{}, err
		}
	}
	if err := w.Close(); err != nil {
		return protocol.FrameResult{}, err
	}
	endpoint := s.api + "/api/v1/devices/" + url.PathEscape(device) + "/frames"
	// Reuse the exact body, including identity and capture time, after a lost response.
	for attempt := 0; ; attempt++ {
		result, retry, err := s.upload(ctx, endpoint, w.FormDataContentType(), body.Bytes(), device, kind, at)
		if err == nil {
			return result, nil
		}
		if !retry || attempt == 2 || ctx.Err() != nil {
			return protocol.FrameResult{}, err
		}
		timer := time.NewTimer(100 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return protocol.FrameResult{}, ctx.Err()
		case <-timer.C:
		}
	}
}

func (s *SnapshotClient) upload(ctx context.Context, endpoint, contentType string, body []byte, device, kind string, at time.Time) (protocol.FrameResult, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return protocol.FrameResult{}, false, err
	}
	req.Header.Set("Content-Type", contentType)
	if s.credentials.Username != "" {
		req.SetBasicAuth(s.credentials.Username, s.credentials.Password)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return protocol.FrameResult{}, true, fmt.Errorf("snapshot HTTP upload failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return protocol.FrameResult{}, resp.StatusCode >= 500 || resp.StatusCode == http.StatusRequestTimeout || resp.StatusCode == http.StatusTooManyRequests, fmt.Errorf("snapshot upload returned HTTP %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, (64<<10)+1))
	if err != nil {
		return protocol.FrameResult{}, true, fmt.Errorf("snapshot response read failed: %w", err)
	}
	if len(raw) > 64<<10 {
		return protocol.FrameResult{}, false, fmt.Errorf("snapshot response is too large")
	}
	var response struct {
		Code *int                  `json:"code"`
		Data *protocol.FrameResult `json:"data"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		return protocol.FrameResult{}, true, fmt.Errorf("invalid snapshot response: %w", err)
	}
	if response.Code == nil || *response.Code != 0 || response.Data == nil || response.Data.DeviceID != device || response.Data.Type != kind || !response.Data.CapturedAt.Equal(at) {
		return protocol.FrameResult{}, false, fmt.Errorf("snapshot response has an invalid result")
	}
	result, err := json.Marshal(protocol.SnapshotResult{SnapshotID: response.Data.SnapshotID})
	if err != nil {
		return protocol.FrameResult{}, false, err
	}
	if _, err := protocol.ParseSnapshotResult(result); err != nil {
		return protocol.FrameResult{}, false, err
	}
	return *response.Data, false, nil
}
