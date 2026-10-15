package auth

import (
	"fmt"
	"net"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Disabled          bool              `yaml:"disabled"`
	PlatformUsername  string            `yaml:"platform_username"`
	PlatformPassword  string            `yaml:"platform_password"`
	DevicePasswords   map[string]string `yaml:"device_passwords"`
	SessionTTLMinutes int               `yaml:"session_ttl_minutes"`
	AllowedOrigins    []string          `yaml:"allowed_origins"`
}

func (c *Config) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("http.auth 必须为对象")
	}
	for i := 0; i < len(node.Content); i += 2 {
		key, value := node.Content[i].Value, node.Content[i+1]
		switch key {
		case "platform_username", "platform_password":
			if value.Tag != "!!str" {
				return fmt.Errorf("http.auth 凭据必须为字符串")
			}
		case "disabled":
			if value.Tag != "!!bool" {
				return fmt.Errorf("http.auth.disabled 必须为布尔值")
			}
		case "session_ttl_minutes":
			if value.Tag != "!!int" {
				return fmt.Errorf("http.auth.session_ttl_minutes 必须为整数")
			}
		case "device_passwords":
			if value.Kind != yaml.MappingNode {
				return fmt.Errorf("http.auth.device_passwords 必须为对象")
			}
			for j := 0; j < len(value.Content); j++ {
				if value.Content[j].Tag != "!!str" {
					return fmt.Errorf("http.auth.device_passwords 身份与密码必须为字符串")
				}
			}
		case "allowed_origins":
			if value.Kind != yaml.SequenceNode {
				return fmt.Errorf("http.auth.allowed_origins 必须为数组")
			}
			for _, v := range value.Content {
				if v.Tag != "!!str" {
					return fmt.Errorf("http.auth.allowed_origins 必须为字符串数组")
				}
			}
		default:
			return fmt.Errorf("http.auth 含未知字段")
		}
	}
	type plain Config
	return node.Decode((*plain)(c))
}

func ValidUsername(value string) bool {
	if len(value) < 1 || len(value) > 64 {
		return false
	}
	for _, c := range value {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-') {
			return false
		}
	}
	return true
}

func ValidPassword(value string) bool { return value != "" && !strings.ContainsAny(value, "\r\n\x00") }

func (c Config) Validate(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("http.addr 必须是 host:port")
	}
	if c.Disabled && host != "127.0.0.1" && host != "::1" && host != "localhost" {
		return fmt.Errorf("http.auth.disabled 仅允许 loopback 监听")
	}
	if c.SessionTTLMinutes < 1 || c.SessionTTLMinutes > 1440 {
		return fmt.Errorf("http.auth.session_ttl_minutes 必须为 1—1440")
	}
	if !c.Disabled {
		if !ValidUsername(c.PlatformUsername) {
			return fmt.Errorf("http.auth.platform_username 必须为 1—64 位 ASCII 字母、数字、下划线或连字符")
		}
		if !ValidPassword(c.PlatformPassword) {
			return fmt.Errorf("http.auth.platform_password 必须非空且不含换行或 NUL")
		}
	}
	for id, password := range c.DevicePasswords {
		if !ValidUsername(id) {
			return fmt.Errorf("http.auth.device_passwords 含非法设备身份")
		}
		if id == c.PlatformUsername {
			return fmt.Errorf("http.auth.device_passwords 与平台身份冲突")
		}
		if !(c.Disabled && password == "") && !ValidPassword(password) {
			return fmt.Errorf("http.auth.device_passwords 含空密码、换行或 NUL")
		}
	}
	seen := map[string]bool{}
	for _, origin := range c.AllowedOrigins {
		key, err := normalizeOrigin(origin)
		if err != nil {
			return fmt.Errorf("http.auth.allowed_origins 含非法 origin")
		}
		if seen[key] {
			return fmt.Errorf("http.auth.allowed_origins 含重复 origin")
		}
		seen[key] = true
	}
	return nil
}
