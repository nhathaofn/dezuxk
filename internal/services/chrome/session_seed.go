package chrome

import (
	"context"
	"errors"
	"strings"
	"time"
)

// ensureGoogleSessionCookies keeps cookie-imported accounts usable by silent
// Chrome operations. Interactive login profiles already contain session
// cookies, so the stored header is only injected when the profile has no valid
// Google session yet.
func ensureGoogleSessionCookies(ctx context.Context, client *CDPClient, storedHeader string) error {
	if client == nil {
		return errors.New("CDP client không hợp lệ")
	}

	cookies, err := client.GetCookies(ctx)
	if err != nil {
		return fmtSessionSeedError("không thể đọc cookie profile", err)
	}
	if _, hasPSID, _ := FilterGoogleCookies(cookies); hasPSID {
		return nil
	}

	if strings.TrimSpace(storedHeader) == "" {
		return errors.New("profile chưa có cookie phiên Google và tài khoản không có cookie lưu trữ")
	}
	if err := client.SetGoogleCookieHeader(ctx, storedHeader); err != nil {
		return fmtSessionSeedError("không thể nạp cookie phiên vào profile", err)
	}
	if _, err := client.EvaluateJS(ctx, "location.reload()"); err != nil {
		return fmtSessionSeedError("không thể tải lại profile sau khi nạp cookie", err)
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(1500 * time.Millisecond):
	}

	cookies, err = client.GetCookies(ctx)
	if err != nil {
		return fmtSessionSeedError("không thể xác nhận cookie sau khi nạp", err)
	}
	if _, hasPSID, _ := FilterGoogleCookies(cookies); !hasPSID {
		return errors.New("cookie đã nạp nhưng profile chưa có phiên Google hợp lệ")
	}
	return nil
}

func fmtSessionSeedError(message string, err error) error {
	if err == nil {
		return errors.New(message)
	}
	return errors.New(message + ": " + err.Error())
}
