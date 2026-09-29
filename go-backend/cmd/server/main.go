// 平台进程入口。
//
// 在 go-backend 目录执行：
//
//	go run ./cmd/server -config config.yaml
//
// 不带参数时，从当前工作目录读取 config.yaml。
// 数据库和截图目录再按配置文件所在位置解析，不跟着工作目录走。
package main

import (
	"flag"
	"log/slog"
	"os"

	"piguard/go-backend/internal/config"
	"piguard/go-backend/internal/server"
)

func main() {
	configPath := flag.String("config", "config.yaml", "配置文件路径")
	flag.Parse()

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))

	cfg, err := config.Load(*configPath)
	if err != nil {
		slog.Error("加载配置失败", "err", err)
		os.Exit(1)
	}
	if err := server.Run(cfg); err != nil {
		slog.Error("平台退出", "err", err)
		os.Exit(1)
	}
}
