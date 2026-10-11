package auth

import (
	"crypto/tls"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func testManager(t *testing.T) *Manager {
	t.Helper()
	m, err := New(Config{PlatformUsername: "platform", PlatformPassword: "test-platform", DevicePasswords: map[string]string{"car-001": "test-device"}, SessionTTLMinutes: 1, AllowedOrigins: []string{"http://127.0.0.1:5173"}}, "127.0.0.1:8080")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Close)
	return m
}

func TestAuthorizationPriorityAndCredentialOwnership(t *testing.T) {
	m := testManager(t)
	s, err := m.CreateSession()
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("GET", "http://127.0.0.1:8080/api/v1/devices", nil)
	r.AddCookie(s.Cookie(r))
	if id, ok := m.Authenticate(r); !ok || id.Mode != "cookie" || id.Role != "platform" {
		t.Fatal("cookie identity missing")
	}
	r.SetBasicAuth("car-001", "test-device")
	if id, ok := m.Authenticate(r); !ok || id.Mode != "basic" || id.Role != "device" {
		t.Fatal("Authorization did not override cookie")
	}
	r.SetBasicAuth("platform", "wrong")
	if _, ok := m.Authenticate(r); ok {
		t.Fatal("invalid Authorization fell back to cookie")
	}
	r.Header.Add("Authorization", "Basic duplicate")
	if _, ok := m.Authenticate(r); ok {
		t.Fatal("duplicate Authorization accepted")
	}
}

func TestSessionsCapacityFixedExpiryRevokeAndClose(t *testing.T) {
	m := testManager(t)
	s, err := m.CreateSession()
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("GET", "http://127.0.0.1:8080/api/v1/auth/session", nil)
	cookie := s.Cookie(r)
	if !cookie.HttpOnly || cookie.Secure || cookie.Domain != "" || cookie.Path != "/api/v1" || cookie.SameSite != http.SameSiteStrictMode || cookie.MaxAge < 58 {
		t.Fatal("incorrect cookie policy")
	}
	r.TLS = &tls.ConnectionState{}
	if !s.Cookie(r).Secure {
		t.Fatal("HTTPS cookie lacks Secure")
	}
	r.AddCookie(cookie)
	expires := s.ExpiresAt
	for range 5 {
		if m.RequestSession(r) != s {
			t.Fatal("session disappeared")
		}
	}
	if !s.ExpiresAt.Equal(expires) {
		t.Fatal("session expiry was extended")
	}
	if !s.ValidCSRF(s.CSRFToken) || s.ValidCSRF("") || s.ValidCSRF("wrong") {
		t.Fatal("CSRF validation failed")
	}
	for range sessionLimit - 1 {
		if _, err := m.CreateSession(); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := m.CreateSession(); !errors.Is(err, ErrCapacity) {
		t.Fatal("session limit not enforced")
	}
	m.Revoke(s)
	select {
	case <-s.Done:
	default:
		t.Fatal("revoke did not notify WS")
	}
	if m.RequestSession(r) != nil {
		t.Fatal("revoked cookie remains valid")
	}
	replacement, err := m.CreateSession()
	if err != nil {
		t.Fatal("capacity not released", err)
	}
	m.Close()
	select {
	case <-replacement.Done:
	default:
		t.Fatal("close did not revoke sessions")
	}
	if _, err := m.CreateSession(); !errors.Is(err, ErrCapacity) {
		t.Fatal("closed manager accepted a session")
	}
	other := testManager(t)
	if other.RequestSession(r) != nil {
		t.Fatal("new process accepted old session")
	}
}

func TestSessionExpiryAndConcurrentRevocation(t *testing.T) {
	m := testManager(t)
	s, err := m.CreateSession()
	if err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	s.timer.Reset(10 * time.Millisecond)
	m.mu.Unlock()
	select {
	case <-s.Done:
	case <-time.After(time.Second):
		t.Fatal("expiry timer did not revoke")
	}
	s, err = m.CreateSession()
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() { defer wg.Done(); m.Revoke(s) }()
	}
	wg.Wait()
	select {
	case <-s.Done:
	default:
		t.Fatal("concurrent revoke failed")
	}
}

func TestOriginAndConfigurationBoundaries(t *testing.T) {
	m := testManager(t)
	for _, tc := range []struct {
		origin  string
		allowed bool
	}{
		{"", true}, {"http://127.0.0.1:8080", true}, {"http://127.0.0.1:5173", true},
		{"http://localhost:5173", false}, {"https://127.0.0.1:5173", false}, {"http://127.0.0.1:5174", false},
		{"null", false}, {"http://127.0.0.1:5173.evil", false}, {"http://[localhost]:5173", false}, {"http://::1:8080", false},
	} {
		r := httptest.NewRequest("GET", "http://127.0.0.1:8080/api/v1/ws", nil)
		if tc.origin != "" {
			r.Header.Set("Origin", tc.origin)
		}
		if _, allowed := m.Origin(r); allowed != tc.allowed {
			t.Errorf("origin %q: %v", tc.origin, allowed)
		}
	}
	r := httptest.NewRequest("GET", "http://127.0.0.1:8080/api/v1/ws", nil)
	r.Header.Add("Origin", "http://127.0.0.1:5173")
	r.Header.Add("Origin", "http://127.0.0.1:8080")
	if _, ok := m.Origin(r); ok {
		t.Fatal("multiple origins accepted")
	}
	for _, mutate := range []func(*Config){
		func(c *Config) { c.PlatformPassword = "" }, func(c *Config) { c.PlatformPassword = "secret\n" },
		func(c *Config) { c.SessionTTLMinutes = 0 }, func(c *Config) { c.DevicePasswords = map[string]string{"platform": "secret"} },
		func(c *Config) { c.AllowedOrigins = []string{"http://[localhost]:5173"} },
		func(c *Config) { c.AllowedOrigins = []string{"http://127.0.0.1:5173", "http://127.0.0.1:5173/"} },
	} {
		c := m.config
		mutate(&c)
		if err := c.Validate("127.0.0.1:8080"); err == nil || strings.Contains(err.Error(), "secret") {
			t.Fatal("invalid config accepted or credential disclosed")
		}
	}
	if err := (Config{Disabled: true, SessionTTLMinutes: 30}).Validate("0.0.0.0:8080"); err == nil {
		t.Fatal("anonymous non-loopback accepted")
	}
}
