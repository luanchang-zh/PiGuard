// Package apperr 存放跨层使用的业务错误和错误码。
// 错误码与《网络交互与 API 规范》第 54 节一致，HTTP 状态码由 handler 决定。
package apperr

import "errors"

const (
	// CodeOK 表示业务成功。HTTP 状态另行返回。
	CodeOK = 0
	// CodeInvalidParams 表示请求字段缺失或超出取值范围。
	CodeInvalidParams = 40001
	// CodeDeviceNotFound 表示 device_id 在平台上不存在。
	CodeDeviceNotFound = 40401
	// CodeCommandNotFound 表示 command_id 不存在。
	CodeCommandNotFound  = 40402
	CodeSnapshotNotFound = 40403
	// CodeCommandDuplicated 表示同一条命令被重复提交。
	CodeCommandDuplicated = 40901
	// CodeConfigVersionConflict 表示配置版本冲突，客户端持有的不是最新期望版本。
	CodeConfigVersionConflict = 40902
	CodeFrameConflict         = 40903
	// CodeDeviceOffline 表示动作类命令在设备离线时被拒绝。
	CodeDeviceOffline = 50301
	// CodeMQTTUnavailable 表示平台当前发不出 MQTT 消息。
	CodeMQTTUnavailable = 50302
	// CodeInternal 表示平台内部错误。
	// 初始化阶段的“接口尚未实现”也先用这个码，响应 message 会写明原因。
	CodeInternal = 50001
)

// ErrNotImplemented 表示这条调用链已经接通，但业务还没写。
// handler 把它转换成 HTTP 501，避免调用方误以为请求已经生效。
var ErrNotImplemented = errors.New("not implemented")

var ErrDeviceNotFound = errors.New("device not found")

var ErrCommandNotFound = errors.New("command not found")
var ErrDeviceOffline = errors.New("device offline")
var ErrMQTTUnavailable = errors.New("MQTT unavailable")
var ErrConfigVersionConflict = errors.New("config version conflict")
var ErrSnapshotNotFound = errors.New("snapshot not found")
var ErrFrameConflict = errors.New("frame identity conflict")

type FrameTooLarge struct{ Field string }

func (e *FrameTooLarge) Error() string { return "frame limit exceeded: " + e.Field }

// ErrPublishUncertain means that the message may already have reached the device.
var ErrPublishUncertain = errors.New("MQTT delivery uncertain")

type InvalidParams struct{ Field string }

func (e *InvalidParams) Error() string { return "invalid parameter: " + e.Field }
