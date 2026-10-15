package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"net/http"
	"sync"
	"time"
)

const CookieName = "piguard_session"
const sessionLimit = 1024

var ErrCapacity = errors.New("session capacity reached")

type Session struct {
	ExpiresAt time.Time
	CSRFToken string
	Done      <-chan struct{}
	token     string
	done      chan struct{}
	timer     *time.Timer
}

type Identity struct {
	Username string
	Role     string
	Mode     string
	Session  *Session
}

type Manager struct {
	config   Config
	origins  map[string]struct{}
	mu       sync.Mutex
	sessions map[string]*Session
	closed   bool
}

func New(c Config, addr string) (*Manager, error) {
	if err := c.Validate(addr); err != nil {
		return nil, err
	}
	// Own configuration copies so callers cannot change credentials after validation.
	passwords := make(map[string]string, len(c.DevicePasswords))
	for id, password := range c.DevicePasswords {
		passwords[id] = password
	}
	c.DevicePasswords = passwords
	m := &Manager{config: c, origins: map[string]struct{}{}, sessions: map[string]*Session{}}
	for _, raw := range c.AllowedOrigins {
		key, _ := normalizeOrigin(raw)
		m.origins[key] = struct{}{}
	}
	return m, nil
}

func (m *Manager) Disabled() bool { return m.config.Disabled }

func sameSecret(a, b string) bool {
	aa, bb := sha256.Sum256([]byte(a)), sha256.Sum256([]byte(b))
	return subtle.ConstantTimeCompare(aa[:], bb[:]) == 1
}

func (m *Manager) Authenticate(r *http.Request) (Identity, bool) {
	if len(r.Header.Values("Authorization")) > 0 {
		if len(r.Header.Values("Authorization")) != 1 {
			return Identity{}, false
		}
		username, password, ok := r.BasicAuth()
		if !ok {
			return Identity{}, false
		}
		if username == m.config.PlatformUsername && sameSecret(password, m.config.PlatformPassword) {
			return Identity{Username: username, Role: "platform", Mode: "basic"}, true
		}
		expected, exists := m.config.DevicePasswords[username]
		if sameSecret(password, expected) && exists {
			return Identity{Username: username, Role: "device", Mode: "basic"}, true
		}
		return Identity{}, false
	}
	s := m.RequestSession(r)
	if s == nil {
		return Identity{}, false
	}
	return Identity{Username: m.config.PlatformUsername, Role: "platform", Mode: "cookie", Session: s}, true
}

func (m *Manager) RequestSession(r *http.Request) *Session {
	var token string
	for _, cookie := range r.Cookies() {
		if cookie.Name == CookieName {
			if token != "" {
				return nil
			}
			token = cookie.Value
		}
	}
	if token == "" {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.sessions[token]
	if s != nil && !time.Now().Before(s.ExpiresAt) {
		m.removeLocked(token)
		return nil
	}
	return s
}

func randomToken() (string, error) {
	var bytes [32]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes[:]), nil
}

func (m *Manager) CreateSession() (*Session, error) {
	token, err := randomToken()
	if err != nil {
		return nil, err
	}
	csrf, err := randomToken()
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now().UTC()
	for key, session := range m.sessions {
		if !now.Before(session.ExpiresAt) {
			m.removeLocked(key)
		}
	}
	if m.closed || len(m.sessions) >= sessionLimit {
		return nil, ErrCapacity
	}
	done := make(chan struct{})
	s := &Session{token: token, CSRFToken: csrf, ExpiresAt: now.Add(time.Duration(m.config.SessionTTLMinutes) * time.Minute), Done: done, done: done}
	m.sessions[token] = s
	s.timer = time.AfterFunc(time.Until(s.ExpiresAt), func() { m.mu.Lock(); defer m.mu.Unlock(); m.removeLocked(token) })
	return s, nil
}

func (m *Manager) removeLocked(token string) {
	if s := m.sessions[token]; s != nil {
		delete(m.sessions, token)
		if s.timer != nil {
			s.timer.Stop()
		}
		close(s.done)
	}
}

func (m *Manager) Revoke(s *Session) {
	if s != nil {
		m.mu.Lock()
		defer m.mu.Unlock()
		m.removeLocked(s.token)
	}
}
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	for token := range m.sessions {
		m.removeLocked(token)
	}
}

func (s *Session) ValidCSRF(value string) bool { return value != "" && sameSecret(value, s.CSRFToken) }

func (s *Session) Cookie(r *http.Request) *http.Cookie {
	return &http.Cookie{Name: CookieName, Value: s.token, Path: "/api/v1", HttpOnly: true, Secure: r.TLS != nil, SameSite: http.SameSiteStrictMode, Expires: s.ExpiresAt, MaxAge: int(time.Until(s.ExpiresAt).Seconds())}
}

func ClearCookie(r *http.Request) *http.Cookie {
	return &http.Cookie{Name: CookieName, Path: "/api/v1", HttpOnly: true, Secure: r.TLS != nil, SameSite: http.SameSiteStrictMode, MaxAge: -1, Expires: time.Unix(1, 0)}
}
