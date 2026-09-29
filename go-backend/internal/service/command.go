package service

import (
	"context"
	"fmt"

	"piguard/go-backend/internal/apperr"
	"piguard/go-backend/internal/model"
	"piguard/go-backend/internal/repo"
)

// Publisher 是服务层看到的 MQTT 发布端口。
// 具体客户端由 mqtt 包实现，服务层不依赖 Paho。
type Publisher interface {
	Publish(ctx context.Context, topic string, qos byte, retained bool, payload []byte) error
}

// CommandService 负责把网页上的控制请求变成命令记录，并在之后通过 MQTT 下发。
type CommandService interface {
	Create(ctx context.Context, deviceID string) error
	Get(ctx context.Context, commandID string) (*model.Command, error)
}

type commandService struct {
	commands repo.CommandRepository
	pub      Publisher
}

func NewCommandService(commands repo.CommandRepository, pub Publisher) CommandService {
	return &commandService{commands: commands, pub: pub}
}

// Create 下一步会生成 command_id、写入 commands，再发布到 car/{device_id}/commands。
func (s *commandService) Create(context.Context, string) error {
	if s.commands == nil || s.pub == nil {
		return fmt.Errorf("命令依赖未注入")
	}
	return apperr.ErrNotImplemented
}

func (s *commandService) Get(ctx context.Context, commandID string) (*model.Command, error) {
	return s.commands.Find(ctx, commandID)
}
