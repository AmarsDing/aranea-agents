package wechatilink

import "errors"

// ErrSessionExpired indicates the iLink login session is no longer valid
// (errcode -14). The connector should surface it so the runtime supervisor
// backs off; re-login (QR scan) writes fresh credentials which triggers a
// runtime reload and a new connector attempt.
var ErrSessionExpired = errors.New("wechat_ilink: session expired, re-login required")

// errcodeSessionExpired is the iLink session-timeout error code.
const errcodeSessionExpired = -14

func isSessionExpired(errcode int) bool {
	return errcode == errcodeSessionExpired
}
