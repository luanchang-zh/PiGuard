// Package config 从 YAML 加载平台配置，再用环境变量覆盖。
// 相对路径按配置文件所在目录解析，因此工作目录不影响数据库和截图的位置。
package config

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
	"piguard/go-backend/internal/auth"
)

// Config 是进程启动所需的全部配置。
type Config struct {
	HTTP     HTTPConfig     `yaml:"http"`
	MQTT     MQTTConfig     `yaml:"mqtt"`
	Database DatabaseConfig `yaml:"database"`
	Storage  StorageConfig  `yaml:"storage"`
	Seed     SeedConfig     `yaml:"seed"`
}

// HTTPConfig 决定 Gin 的监听地址，例如 127.0.0.1:8080 或 0.0.0.0:8080。
type HTTPConfig struct {
	Addr string      `yaml:"addr"`
	Auth auth.Config `yaml:"auth"`
}

// MQTTConfig 是连接 Mosquitto 的参数。连不上时进程退出。
type MQTTConfig struct {
	Broker                string `yaml:"broker"`
	ClientID              string `yaml:"client_id"`
	Username              string `yaml:"username"`
	Password              string `yaml:"password"`
	ConnectTimeoutSeconds int    `yaml:"connect_timeout_seconds"`
	// ConnectTimeout 由 ConnectTimeoutSeconds 换算，不从 YAML 直接读取。
	ConnectTimeout time.Duration `yaml:"-"`
}

func (m *MQTTConfig) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("mqtt 必须为对象")
	}
	for i := 0; i < len(node.Content); i += 2 {
		if (node.Content[i].Value == "username" || node.Content[i].Value == "password") && node.Content[i+1].Tag != "!!str" {
			return fmt.Errorf("mqtt 凭据必须为字符串")
		}
	}
	type plain MQTTConfig
	return node.Decode((*plain)(m))
}

// DatabaseConfig 指向 SQLite 文件。父目录不存在时会在启动时创建。
type DatabaseConfig struct {
	SQLitePath string `yaml:"sqlite_path"`
}

// StorageConfig 是摄像头 JPEG 的根目录。表里只记录相对这个目录的路径或绝对路径。
type StorageConfig struct {
	SnapshotDir string `yaml:"snapshot_dir"`
}

// SeedConfig 是首次启动时登记的演示设备。已有同名设备时不会覆盖。
type SeedConfig struct {
	DeviceID        string `yaml:"device_id"`
	Name            string `yaml:"name"`
	Mode            string `yaml:"mode"`
	SoftwareVersion string `yaml:"software_version"`
}

// Load 读取配置文件，并按下面的环境变量覆盖非空值：
//
//	PIGUARD_HTTP_ADDR
//	PIGUARD_MQTT_BROKER
//	PIGUARD_MQTT_CLIENT_ID
//	PIGUARD_MQTT_USERNAME
//	PIGUARD_MQTT_PASSWORD
//	PIGUARD_MQTT_CONNECT_TIMEOUT_SECONDS
//	PIGUARD_DATABASE_SQLITE_PATH
//	PIGUARD_STORAGE_SNAPSHOT_DIR
//	PIGUARD_SEED_DEVICE_ID
//	PIGUARD_SEED_NAME
//	PIGUARD_SEED_MODE
//	PIGUARD_SEED_SOFTWARE_VERSION
func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件 %s: %w", path, err)
	}

	cfg := Config{HTTP: HTTPConfig{Auth: auth.Config{SessionTTLMinutes: 30}}}
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("解析配置文件 %s: 字段类型或 YAML 格式错误", path)
	}
	if err := applyEnv(&cfg); err != nil {
		return nil, err
	}
	applyDefaults(&cfg)

	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("解析配置文件路径: %w", err)
	}
	baseDir := filepath.Dir(abs)
	cfg.Database.SQLitePath = resolve(baseDir, cfg.Database.SQLitePath)
	cfg.Storage.SnapshotDir = resolve(baseDir, cfg.Storage.SnapshotDir)

	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// applyEnv 只用非空环境变量覆盖文件配置。
// 容器里如果声明了变量但值为空，仍然保留 YAML 中的内容。
func applyEnv(cfg *Config) error {
	setString(&cfg.HTTP.Addr, "PIGUARD_HTTP_ADDR")
	setString(&cfg.HTTP.Auth.PlatformUsername, "PIGUARD_HTTP_PLATFORM_USERNAME")
	setString(&cfg.HTTP.Auth.PlatformPassword, "PIGUARD_HTTP_PLATFORM_PASSWORD")
	if value := os.Getenv("PIGUARD_HTTP_AUTH_DISABLED"); value != "" {
		if value != "true" && value != "false" {
			return fmt.Errorf("PIGUARD_HTTP_AUTH_DISABLED 必须为 true 或 false")
		}
		cfg.HTTP.Auth.Disabled = value == "true"
	}
	if value := os.Getenv("PIGUARD_HTTP_SESSION_TTL_MINUTES"); value != "" {
		n, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("PIGUARD_HTTP_SESSION_TTL_MINUTES 必须为整数")
		}
		cfg.HTTP.Auth.SessionTTLMinutes = n
	}
	if value := os.Getenv("PIGUARD_HTTP_DEVICE_PASSWORDS"); value != "" {
		var passwords map[string]string
		if err := json.Unmarshal([]byte(value), &passwords); err != nil || passwords == nil {
			return fmt.Errorf("PIGUARD_HTTP_DEVICE_PASSWORDS 必须为设备身份到密码的 JSON 对象")
		}
		cfg.HTTP.Auth.DevicePasswords = passwords
	}
	if value := os.Getenv("PIGUARD_HTTP_ALLOWED_ORIGINS"); value != "" {
		var origins []string
		if err := json.Unmarshal([]byte(value), &origins); err != nil || origins == nil {
			return fmt.Errorf("PIGUARD_HTTP_ALLOWED_ORIGINS 必须为 origin 的 JSON 数组")
		}
		cfg.HTTP.Auth.AllowedOrigins = origins
	}
	setString(&cfg.MQTT.Broker, "PIGUARD_MQTT_BROKER")
	setString(&cfg.MQTT.ClientID, "PIGUARD_MQTT_CLIENT_ID")
	setString(&cfg.MQTT.Username, "PIGUARD_MQTT_USERNAME")
	setString(&cfg.MQTT.Password, "PIGUARD_MQTT_PASSWORD")
	setString(&cfg.Database.SQLitePath, "PIGUARD_DATABASE_SQLITE_PATH")
	setString(&cfg.Storage.SnapshotDir, "PIGUARD_STORAGE_SNAPSHOT_DIR")
	setString(&cfg.Seed.DeviceID, "PIGUARD_SEED_DEVICE_ID")
	setString(&cfg.Seed.Name, "PIGUARD_SEED_NAME")
	setString(&cfg.Seed.Mode, "PIGUARD_SEED_MODE")
	setString(&cfg.Seed.SoftwareVersion, "PIGUARD_SEED_SOFTWARE_VERSION")

	if password := os.Getenv("PIGUARD_HTTP_SEED_DEVICE_PASSWORD"); password != "" {
		if cfg.HTTP.Auth.DevicePasswords == nil {
			cfg.HTTP.Auth.DevicePasswords = map[string]string{}
		}
		cfg.HTTP.Auth.DevicePasswords[cfg.Seed.DeviceID] = password
	}
	value, ok := os.LookupEnv("PIGUARD_MQTT_CONNECT_TIMEOUT_SECONDS")
	if !ok || value == "" {
		return nil
	}
	seconds, err := strconv.Atoi(value)
	if err != nil || seconds <= 0 {
		return fmt.Errorf("环境变量 PIGUARD_MQTT_CONNECT_TIMEOUT_SECONDS 必须是正整数")
	}
	cfg.MQTT.ConnectTimeoutSeconds = seconds
	return nil
}

func setString(dst *string, key string) {
	value, ok := os.LookupEnv(key)
	if ok && value != "" {
		*dst = value
	}
}

// applyDefaults 补上演示设备的模式、版本，以及 MQTT 连接超时。
func applyDefaults(cfg *Config) {
	if cfg.Seed.Mode == "" {
		cfg.Seed.Mode = "mock"
	}
	if cfg.Seed.SoftwareVersion == "" {
		cfg.Seed.SoftwareVersion = "0.1.0"
	}
	if cfg.MQTT.ConnectTimeoutSeconds <= 0 {
		cfg.MQTT.ConnectTimeoutSeconds = 5
	}
	cfg.MQTT.ConnectTimeout = time.Duration(cfg.MQTT.ConnectTimeoutSeconds) * time.Second
}

// resolve 把相对路径改到配置文件所在目录下。绝对路径保持不变。
func resolve(baseDir, path string) string {
	if path == "" || filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(baseDir, path)
}

// Validate 在连数据库和 Broker 之前拦截明显的配置错误。
func (c *Config) Validate() error {
	if c == nil {
		return fmt.Errorf("配置不能为空")
	}
	if _, _, err := net.SplitHostPort(c.HTTP.Addr); err != nil {
		return fmt.Errorf("http.addr 必须是 host:port，当前是 %q", c.HTTP.Addr)
	}
	broker, err := url.Parse(c.MQTT.Broker)
	if err != nil || broker.Hostname() == "" || broker.User != nil || broker.RawQuery != "" || broker.ForceQuery || broker.Fragment != "" || (broker.Scheme != "tcp" && broker.Scheme != "ssl" && broker.Scheme != "ws" && broker.Scheme != "wss") {
		return fmt.Errorf("mqtt.broker 必须为不含凭据的 MQTT 地址，例如 tcp://127.0.0.1:1883")
	}
	if c.MQTT.ClientID == "" {
		return fmt.Errorf("mqtt.client_id 不能为空")
	}
	if c.Database.SQLitePath == "" {
		return fmt.Errorf("database.sqlite_path 不能为空")
	}
	if c.Storage.SnapshotDir == "" {
		return fmt.Errorf("storage.snapshot_dir 不能为空")
	}
	if c.Seed.DeviceID == "" {
		return fmt.Errorf("seed.device_id 不能为空")
	}
	if c.Seed.Name == "" {
		return fmt.Errorf("seed.name 不能为空")
	}
	if err := c.HTTP.Auth.Validate(c.HTTP.Addr); err != nil {
		return err
	}
	if !c.HTTP.Auth.Disabled {
		if !auth.ValidUsername(c.Seed.DeviceID) || !auth.ValidPassword(c.HTTP.Auth.DevicePasswords[c.Seed.DeviceID]) {
			return fmt.Errorf("http.auth.device_passwords 必须包含 seed.device_id 的合法凭据")
		}
		if c.MQTT.Username == "" || c.MQTT.Password == "" {
			return fmt.Errorf("mqtt.username/password 默认认证模式必须完整配置")
		}
	}
	if c.MQTT.Username != "" || c.MQTT.Password != "" {
		if !auth.ValidUsername(c.MQTT.Username) || !auth.ValidPassword(c.MQTT.Password) {
			return fmt.Errorf("mqtt.username/password 必须配对且合法")
		}
		if _, exists := c.HTTP.Auth.DevicePasswords[c.MQTT.Username]; exists || c.MQTT.Username == c.Seed.DeviceID {
			return fmt.Errorf("mqtt.username 平台与设备身份不能重用")
		}
	}
	return nil
}
