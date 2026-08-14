package samsung

import (
	"context"
	"errors"
	"strings"

	"github.com/galaxytty/galaxytty/internal/domain"
)

const (
	MessagesPackage = "com.samsung.android.messaging"
	smsRoleName     = "android.app.role.SMS"
)

func SamsungMessagesIsDefault(output string) bool {
	for _, holder := range strings.Fields(output) {
		if holder == MessagesPackage {
			return true
		}
	}
	return false
}

func ensureDefaultSMSHandler(ctx context.Context, device domain.Device) error {
	output, err := device.Shell(ctx, "cmd", "role", "get-role-holders", smsRoleName)
	if err != nil {
		return safeControllerError{kind: domain.ErrSamsungMessagesNotDefault, cause: err}
	}
	if !SamsungMessagesIsDefault(string(output)) {
		return safeControllerError{kind: domain.ErrSamsungMessagesNotDefault, cause: errors.New("required role holder is absent")}
	}
	return nil
}
