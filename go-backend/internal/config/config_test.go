package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// clearEnv 把本包认识的环境变量置空，避免开发机上的变量影响断言。
func clearEnv(t *testing.T) {
	t.Helper()
	keys := []string{
		"PIGUARD_HTTP_ADDR",
		"PIGUARD_MQTT_BROKER",
		"PIGUARD_MQTT_CLIENT_ID",
		"PIGUARD_MQTT_USERNAME",
		"PIGUARD_MQTT_PASSWORD",
		"PIGUARD_MQTT_CONNECT_TIMEOUT_SECONDS",
		"PIGUARD_DATABASE_SQLITE_PATH",
		"PIGUARD_STORAGE_SNAPSHOT_DIR",
		"PIGUARD_SEED_DEVICE_ID",
		"PIGUARD_SEED_NAME",
		"PIGUARD_SEED_MODE",
		"PIGUARD_SEED_SOFTWARE_VERSION",
	}
	for _, key := range keys {
		t.Setenv(key, "")
	}
}

func TestLoadAndEnvOverride(t *testing.T) {
	clearEnv(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := []byte(`
http:
  addr: 127.0.0.1:8080
mqtt:
  broker: tcp://127.0.0.1:1883
  client_id: piguard-backend
  connect_timeout_seconds: 5
database:
  sqlite_path: data/piguard.db
storage:
  snapshot_dir: data/snapshots
seed:
  device_id: car-001
  name: Demo Car
`)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PIGUARD_HTTP_ADDR", "0.0.0.0:9090")
	t.Setenv("PIGUARD_MQTT_BROKER", "tcp://mosquitto:1883")
	t.Setenv("PIGUARD_DATABASE_SQLITE_PATH", "/var/lib/piguard/piguard.db")

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTP.Addr != "0.0.0.0:9090" {
		t.Fatalf("监听地址没有被环境变量覆盖，得到 %s", cfg.HTTP.Addr)
	}
	if cfg.MQTT.Broker != "tcp://mosquitto:1883" {
		t.Fatalf("Broker 地址没有被环境变量覆盖，得到 %s", cfg.MQTT.Broker)
	}
	if cfg.Database.SQLitePath != "/var/lib/piguard/piguard.db" {
		t.Fatalf("绝对路径应保持原样，得到 %s", cfg.Database.SQLitePath)
	}
	if cfg.Storage.SnapshotDir != filepath.Join(dir, "data", "snapshots") {
		t.Fatalf("截图目录应相对配置文件解析，得到 %s", cfg.Storage.SnapshotDir)
	}
	if cfg.MQTT.ConnectTimeout != 5*time.Second {
		t.Fatalf("连接超时应为 5 秒，得到 %s", cfg.MQTT.ConnectTimeout)
	}
	if cfg.Seed.Mode != "mock" || cfg.Seed.SoftwareVersion != "0.1.0" {
		t.Fatalf("未填写的种子字段应使用默认值，得到 mode=%s version=%s", cfg.Seed.Mode, cfg.Seed.SoftwareVersion)
	}
}

func TestLoadRejectsBrokerWithoutScheme(t *testing.T) {
	clearEnv(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := []byte(`
http:
  addr: 127.0.0.1:8080
mqtt:
  broker: 127.0.0.1:1883
  client_id: piguard-backend
database:
  sqlite_path: data/piguard.db
storage:
  snapshot_dir: data/snapshots
seed:
  device_id: car-001
  name: Demo Car
`)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("缺少协议的 Broker 地址应该加载失败")
	}
}

func TestCommittedConfigParses(t *testing.T) {
	clearEnv(t)
	cfg, err := Load(filepath.Join("..", "..", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Seed.DeviceID != "car-001" {
		t.Fatalf("仓库内配置的演示设备应为 car-001，得到 %s", cfg.Seed.DeviceID)
	}
	if cfg.HTTP.Addr != "127.0.0.1:8080" {
		t.Fatalf("仓库内配置应监听本机 8080，得到 %s", cfg.HTTP.Addr)
	}
}
