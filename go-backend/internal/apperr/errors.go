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
	CodeCommandNotFound = 40402
	// CodeCommandDuplicated 表示同一条命令被重复提交。
	CodeCommandDuplicated = 40901
	// CodeConfigVersionConflict 表示配置版本冲突，客户端持有的不是最新期望版本。
	CodeConfigVersionConflict = 40902
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
