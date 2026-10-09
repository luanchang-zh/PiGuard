package service

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"
	"piguard/go-backend/internal/apperr"
	"piguard/go-backend/internal/db"
	"piguard/go-backend/internal/protocol"
	"piguard/go-backend/internal/repo"
)

type configPublisher struct {
	available atomic.Bool
	called    chan protocol.ConfigMessage
	fn        func(context.Context, protocol.ConfigMessage) error
}

func (p *configPublisher) Available() bool { return p.available.Load() }
func (p *configPublisher) Publish(ctx context.Context, topic string, qos byte, retained bool, raw []byte) error {
	c, err := protocol.ParseConfigMessage(topic, raw)
	if err != nil {
		return err
	}
	if qos != 1 || !retained {
		return errors.New("incorrect config delivery options")
	}
	p.called <- *c
	if p.fn != nil {
		return p.fn(ctx, *c)
	}
	return nil
}

type configFixture struct {
	s       *ConfigManager
	r       repo.ConfigRepository
	devices repo.DeviceRepository
	p       *configPublisher
	g       *gorm.DB
	path    string
}

func newConfigFixture(t *testing.T) *configFixture {
	t.Helper()
	f := &configFixture{path: filepath.Join(t.TempDir(), "config.db"), p: &configPublisher{called: make(chan protocol.ConfigMessage, 256)}}
	var err error
	f.g, err = db.Open(f.path)
	if err != nil {
		t.Fatal(err)
	}
	sql, _ := f.g.DB()
	t.Cleanup(func() { sql.Close() })
	f.r = repo.NewConfigRepository(f.g)
	f.devices = repo.NewDeviceRepository(f.g)
	if _, err = NewDeviceService(f.devices, f.r).Seed(context.Background(), SeedInput{DeviceID: "car-001", Name: "Demo", Mode: "mock", SoftwareVersion: "0.1.0"}); err != nil {
		t.Fatal(err)
	}
	f.p.available.Store(true)
	f.s = NewConfigService(f.r, f.p, f.devices)
	t.Cleanup(f.s.Close)
	return f
}
func (f *configFixture) start(t *testing.T) {
	t.Helper()
	if err := f.s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
}
func configRequest(version int, patch string) protocol.ConfigRequest {
	return protocol.ConfigRequest{ExpectedVersion: version, Rules: json.RawMessage(patch)}
}
func configSuccess(version int) protocol.ConfigAck {
	at := time.Now().UTC()
	return protocol.ConfigAck{SchemaVersion: 1, DeviceID: "car-001", ConfigVersion: version, Status: "success", AppliedAt: &at}
}
func configIngest(s *ConfigManager, a protocol.ConfigAck) error {
	raw, _ := json.Marshal(a)
	return s.IngestAck(context.Background(), "car/"+a.DeviceID+"/config-acks", raw)
}
func nextConfig(t *testing.T, p *configPublisher) protocol.ConfigMessage {
	t.Helper()
	select {
	case c := <-p.called:
		return c
	case <-time.After(3 * time.Second):
		t.Fatal("config not published")
	}
	return protocol.ConfigMessage{}
}
func configView(t *testing.T, s *ConfigManager, status string) *protocol.ConfigView {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		v, err := s.Get(context.Background(), "car-001")
		if err != nil {
			t.Fatal(err)
		}
		if v.Status == status {
			return v
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("config did not become", status)
	return nil
}

func TestConfigFastAckFirstResultAndVersionFacts(t *testing.T) {
	f := newConfigFixture(t)
	ack := configSuccess(2)
	f.p.fn = func(ctx context.Context, c protocol.ConfigMessage) error {
		row, err := f.r.Snapshot(ctx, c.DeviceID)
		if err != nil || row.Config.DesiredVersion != 2 || row.Revision.ConfigVersion != 2 {
			return errors.New("publish preceded commit")
		}
		return configIngest(f.s, ack)
	}
	f.start(t)
	v := configView(t, f.s, "pending")
	if v.DesiredVersion != 1 || v.ReportedVersion != 0 || v.AckAt != nil || v.AppliedAt != nil {
		t.Fatal(v)
	}
	version, err := f.s.Update(context.Background(), "car-001", configRequest(1, `{"obstacle":{"warning_distance_m":20}}`))
	if err != nil || version != 2 {
		t.Fatal(version, err)
	}
	c := nextConfig(t, f.p)
	if c.ConfigVersion != 2 || c.Rules.Obstacle.WarningDistanceM != 20 || c.Rules.Obstacle.DangerDistanceM != 8 {
		t.Fatal(c)
	}
	v = configView(t, f.s, "success")
	if v.ReportedVersion != 2 || v.AckAt == nil || v.AppliedAt == nil || v.Error != nil {
		t.Fatal(v)
	}
	firstAckAt := *v.AckAt
	if err = configIngest(f.s, ack); err != nil {
		t.Fatal(err)
	}
	conflict := ack
	conflict.Status = "failed"
	conflict.AppliedAt = nil
	conflict.Error = &protocol.ConfigError{Code: "BAD", Message: "bad"}
	if err = configIngest(f.s, conflict); err == nil {
		t.Fatal("conflicting terminal Ack accepted")
	}
	if err = configIngest(f.s, configSuccess(1)); err != nil {
		t.Fatal(err)
	}
	v = configView(t, f.s, "success")
	if v.ReportedVersion != 2 || !v.AckAt.Equal(firstAckAt) {
		t.Fatal("terminal state regressed", v)
	}
	for _, bad := range []protocol.ConfigAck{configSuccess(99), {SchemaVersion: 1, DeviceID: "car-002", ConfigVersion: 2, Status: "success", AppliedAt: ack.AppliedAt}} {
		if err = configIngest(f.s, bad); err == nil {
			t.Fatal("unknown Ack accepted")
		}
	}
	d, err := f.devices.Find(context.Background(), "car-001")
	if err != nil || d.Online || d.ReportedConfigVersion != 0 || d.LastSeenAt != nil {
		t.Fatal("config Ack modified status facts", d, err)
	}
}

func TestConfigConcurrentVersionsAndOldAck(t *testing.T) {
	f := newConfigFixture(t)
	f.start(t)
	var ok, conflicts atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := f.s.Update(context.Background(), "car-001", configRequest(1, `{"obstacle":{"warning_distance_m":20}}`))
			if err == nil {
				ok.Add(1)
			} else if errors.Is(err, apperr.ErrConfigVersionConflict) {
				conflicts.Add(1)
			} else {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if ok.Load() != 1 || conflicts.Load() != 7 {
		t.Fatal(ok.Load(), conflicts.Load())
	}
	nextConfig(t, f.p)
	version, err := f.s.Update(context.Background(), "car-001", configRequest(2, `{"sharp_turn":{"min_speed_kmh":0}}`))
	if err != nil || version != 3 {
		t.Fatal(version, err)
	}
	nextConfig(t, f.p)
	if err = configIngest(f.s, configSuccess(2)); err != nil {
		t.Fatal(err)
	}
	v := configView(t, f.s, "pending")
	if v.DesiredVersion != 3 || v.ReportedVersion != 2 || v.Rules.SharpTurn.MinSpeedKmh != 0 || v.Rules.Obstacle.WarningDistanceM != 20 {
		t.Fatal(v)
	}
	failed := protocol.ConfigAck{SchemaVersion: 1, DeviceID: "car-001", ConfigVersion: 3, Status: "failed", Error: &protocol.ConfigError{Code: "INVALID_CONFIG", Message: "device rejected"}}
	if err = configIngest(f.s, failed); err != nil {
		t.Fatal(err)
	}
	v = configView(t, f.s, "failed")
	if v.ReportedVersion != 2 || v.AppliedAt != nil || v.Error == nil || v.Error.Code != "INVALID_CONFIG" {
		t.Fatal(v)
	}
	if err = configIngest(f.s, configSuccess(3)); err == nil {
		t.Fatal("failed result overwritten")
	}
}

func TestConfigPublishOrderWhileNewVersionArrives(t *testing.T) {
	f := newConfigFixture(t)
	entered := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan int, 8)
	f.p.fn = func(ctx context.Context, c protocol.ConfigMessage) error {
		if c.ConfigVersion == 2 {
			close(entered)
			select {
			case <-release:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		finished <- c.ConfigVersion
		return nil
	}
	f.start(t)
	if _, err := f.s.Update(context.Background(), "car-001", configRequest(1, `{"obstacle":{"warning_distance_m":20}}`)); err != nil {
		t.Fatal(err)
	}
	<-entered
	for _, version := range []int{2, 3} {
		if _, err := f.s.Update(context.Background(), "car-001", configRequest(version, `{"temperature":{"trigger_c":40}}`)); err != nil {
			t.Fatal(err)
		}
	}
	// Restore while an old publish is in flight must not introduce another sender.
	if err := f.s.Restore(context.Background()); err != nil {
		t.Fatal(err)
	}
	close(release)
	for _, want := range []int{2, 4} {
		select {
		case got := <-finished:
			if got != want {
				t.Fatal(got, want)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("sender stuck")
		}
	}
	select {
	case got := <-finished:
		if got < 4 {
			t.Fatal("old retained replaced latest", got)
		}
	default:
	}
	v := configView(t, f.s, "pending")
	if v.DesiredVersion != 4 || v.ReportedVersion != 0 {
		t.Fatal(v)
	}
}

func TestConfigRejectionAndAtomicRollback(t *testing.T) {
	f := newConfigFixture(t)
	f.start(t)
	f.p.available.Store(false)
	if _, err := f.s.Update(context.Background(), "car-001", configRequest(1, `{"obstacle":{"warning_distance_m":20}}`)); !errors.Is(err, apperr.ErrMQTTUnavailable) {
		t.Fatal(err)
	}
	f.p.available.Store(true)
	if _, err := f.s.Update(context.Background(), "unknown", configRequest(1, `{"obstacle":{"warning_distance_m":20}}`)); !errors.Is(err, apperr.ErrDeviceNotFound) {
		t.Fatal(err)
	}
	if _, err := f.s.Update(context.Background(), "car-001", configRequest(1, `{"obstacle":{"danger_distance_m":20}}`)); err == nil {
		t.Fatal("invalid merged config saved")
	}
	if err := f.g.Exec("CREATE TRIGGER reject_config_revision BEFORE INSERT ON config_revisions WHEN NEW.config_version = 2 BEGIN SELECT RAISE(ABORT, 'simulated write failure'); END").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.Update(context.Background(), "car-001", configRequest(1, `{"obstacle":{"warning_distance_m":20}}`)); err == nil {
		t.Fatal("database failure hidden")
	}
	v := configView(t, f.s, "pending")
	if v.DesiredVersion != 1 || v.Rules.Obstacle.WarningDistanceM != 15 {
		t.Fatal("transaction leaked config", v)
	}
	select {
	case <-f.p.called:
		t.Fatal("rejected write published")
	default:
	}
}

func TestConfigRestartAndReconnectRestoreLatestWithoutNewVersion(t *testing.T) {
	f := newConfigFixture(t)
	f.p.fn = func(context.Context, protocol.ConfigMessage) error { return apperr.ErrPublishUncertain }
	f.start(t)
	if _, err := f.s.Update(context.Background(), "car-001", configRequest(1, `{"obstacle":{"warning_distance_m":20}}`)); err != nil {
		t.Fatal(err)
	}
	original := nextConfig(t, f.p)
	configView(t, f.s, "pending")
	f.s.Close()
	sql, _ := f.g.DB()
	sql.Close()
	g, err := db.Open(f.path)
	if err != nil {
		t.Fatal(err)
	}
	sql2, _ := g.DB()
	defer sql2.Close()
	p := &configPublisher{called: make(chan protocol.ConfigMessage, 16)}
	p.available.Store(true)
	s := NewConfigService(repo.NewConfigRepository(g), p, repo.NewDeviceRepository(g))
	if err = s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err = s.Restore(context.Background()); err != nil {
		t.Fatal(err)
	}
	restored := nextConfig(t, p)
	if restored.ConfigVersion != 2 || restored.Rules != original.Rules || !restored.IssuedAt.Equal(original.IssuedAt) {
		t.Fatal(original, restored)
	}
	if err = configIngest(s, configSuccess(2)); err != nil {
		t.Fatal(err)
	}
	if err = s.Restore(context.Background()); err != nil {
		t.Fatal(err)
	}
	restored = nextConfig(t, p)
	if restored.ConfigVersion != 2 {
		t.Fatal(restored)
	}
	v := configView(t, s, "success")
	if v.DesiredVersion != 2 || v.ReportedVersion != 2 {
		t.Fatal(v)
	}
	s.Close()
	sql2.Close()
	g3, err := db.Open(f.path)
	if err != nil {
		t.Fatal(err)
	}
	sql3, _ := g3.DB()
	defer sql3.Close()
	p3 := &configPublisher{called: make(chan protocol.ConfigMessage, 16)}
	p3.available.Store(true)
	s3 := NewConfigService(repo.NewConfigRepository(g3), p3, repo.NewDeviceRepository(g3))
	if err = s3.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer s3.Close()
	if err = s3.Restore(context.Background()); err != nil {
		t.Fatal(err)
	}
	nextConfig(t, p3)
	persisted := configView(t, s3, "success")
	if persisted.ReportedVersion != 2 || persisted.AckAt == nil || !persisted.AckAt.Equal(*v.AckAt) || !persisted.AppliedAt.Equal(*v.AppliedAt) {
		t.Fatal("completed result lost on restart", persisted)
	}
}

func TestConfigSeedRestoreAndCloseCancelsNetworkWait(t *testing.T) {
	f := newConfigFixture(t)
	entered := make(chan struct{})
	f.p.fn = func(ctx context.Context, c protocol.ConfigMessage) error {
		close(entered)
		<-ctx.Done()
		return ctx.Err()
	}
	f.start(t)
	if err := f.s.Restore(context.Background()); err != nil {
		t.Fatal(err)
	}
	seed := nextConfig(t, f.p)
	if seed.ConfigVersion != 1 || seed.IssuedAt.IsZero() {
		t.Fatal(seed)
	}
	<-entered
	done := make(chan struct{})
	go func() { f.s.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("close did not cancel sender")
	}
	if _, err := f.s.Update(context.Background(), "car-001", configRequest(1, `{"obstacle":{"warning_distance_m":20}}`)); !errors.Is(err, apperr.ErrMQTTUnavailable) {
		t.Fatal(err)
	}
	if err := f.s.Restore(context.Background()); err == nil {
		t.Fatal("closed manager accepted restore")
	}
	if configView(t, f.s, "pending").ReportedVersion != 0 {
		t.Fatal("seed was confirmed without Ack")
	}
}
