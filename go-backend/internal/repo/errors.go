// Package repo 负责数据库读写。
// 具体实现通过接口注入到 service。这里不做权限、阈值和命令状态判断。
package repo

import "errors"

// ErrNotFound 表示按业务主键没有查到行。
// 是否变成 HTTP 404 由上层决定，仓储只说明“没有这条记录”。
var ErrNotFound = errors.New("not found")

// ErrDuplicate 表示 event_id 唯一约束已经存在，调用方需要比较内容后再决定是否忽略。
var ErrDuplicate = errors.New("duplicate")
