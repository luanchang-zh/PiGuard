package auth

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

func normalizeOrigin(raw string) (string, error) {
	if strings.Contains(raw, "#") {
		return "", fmt.Errorf("invalid origin")
	}
	u, err := url.Parse(raw)
	if err != nil || u == nil {
		return "", fmt.Errorf("invalid origin")
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || (u.Path != "" && u.Path != "/") || u.RawPath != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" {
		return "", fmt.Errorf("invalid origin")
	}
	// net/url accepts bracketed DNS names and unbracketed IPv6 authorities.
	// Check the original authority before Hostname strips its bracket structure.
	if strings.HasPrefix(u.Host, "[") {
		end := strings.IndexByte(u.Host, ']')
		if end < 0 || !strings.Contains(u.Host[1:end], ":") || net.ParseIP(u.Host[1:end]) == nil {
			return "", fmt.Errorf("invalid IPv6 authority")
		}
		suffix := u.Host[end+1:]
		if suffix != "" && (suffix[0] != ':' || strings.Count(suffix, ":") != 1) {
			return "", fmt.Errorf("invalid IPv6 port")
		}
	} else if strings.ContainsAny(u.Host, "[]") || strings.Count(u.Host, ":") > 1 {
		return "", fmt.Errorf("invalid authority")
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return "", fmt.Errorf("invalid host")
	}
	if net.ParseIP(host) == nil {
		for _, label := range strings.Split(host, ".") {
			if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
				return "", fmt.Errorf("invalid host")
			}
			for _, c := range label {
				if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
					return "", fmt.Errorf("invalid host")
				}
			}
		}
	}
	port := u.Port()
	if port == "" {
		if strings.HasSuffix(u.Host, ":") {
			return "", fmt.Errorf("invalid port")
		}
		port = "80"
		if u.Scheme == "https" {
			port = "443"
		}
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return "", fmt.Errorf("invalid port")
	}
	return u.Scheme + "://" + net.JoinHostPort(host, strconv.Itoa(n)), nil
}

// Origin checks the direct request authority; forwarded headers are not trusted.
func (m *Manager) Origin(r *http.Request) (string, bool) {
	values := r.Header.Values("Origin")
	if len(values) == 0 {
		return "", true
	}
	if len(values) != 1 {
		return "", false
	}
	raw := values[0]
	key, err := normalizeOrigin(raw)
	if err != nil {
		return "", false
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	own, err := normalizeOrigin(scheme + "://" + r.Host)
	if err == nil && key == own {
		return raw, true
	}
	_, ok := m.origins[key]
	return raw, ok
}
