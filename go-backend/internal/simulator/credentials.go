package simulator

import (
	"fmt"
	"piguard/go-backend/internal/auth"
)

type Credentials struct{ Username, Password string }

func (c Credentials) Validate(device string, allowEmpty bool) error {
	if !auth.ValidUsername(device) {
		return fmt.Errorf("device 必须为合法 ASCII 身份")
	}
	if c.Username == "" && c.Password == "" && allowEmpty {
		return nil
	}
	if c.Username != device || !auth.ValidUsername(c.Username) || !auth.ValidPassword(c.Password) {
		return fmt.Errorf("设备凭据必须配对，用户名必须等于 device")
	}
	return nil
}
