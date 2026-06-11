package wechat

import (
	"fmt"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

const (
	configReason     = "WECHAT_CONFIG"
	credentialReason = "WECHAT_CREDENTIAL"
	protocolReason   = "WECHAT_PROTOCOL"
)

var (
	errBadSignature           = kerrors.BadRequest(protocolReason, "wechat: bad signature")
	errEmptyEchostr           = kerrors.BadRequest(protocolReason, "wechat: empty echostr")
	errUnsupportedMsgType     = kerrors.BadRequest(protocolReason, "wechat: unsupported msg type")
	errEmptyContent           = kerrors.BadRequest(protocolReason, "wechat: empty content")
	errAppCredentialsRequired = kerrors.BadRequest(credentialReason, "wechat: app_id and app_secret required")
	errOpenIDRequired         = kerrors.BadRequest(protocolReason, "wechat: open_id required")
)

func wechatAPIError(prefix string, msg string) error {
	return kerrors.InternalServer(protocolReason, fmt.Sprintf("%s: %s", prefix, msg))
}

func wechatParseError(prefix string, err error) error {
	return kerrors.InternalServer(protocolReason, fmt.Sprintf("%s: %v", prefix, err))
}
