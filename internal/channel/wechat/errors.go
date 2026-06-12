package wechat

import (
	"fmt"

	"aranea-agents/pkg/apierror"
)

const (
	configReason     = "WECHAT_CONFIG"
	credentialReason = "WECHAT_CREDENTIAL"
	protocolReason   = "WECHAT_PROTOCOL"
)

var (
	errBadSignature           = apierror.BadRequest(protocolReason, "wechat: bad signature")
	errEmptyEchostr           = apierror.BadRequest(protocolReason, "wechat: empty echostr")
	errUnsupportedMsgType     = apierror.BadRequest(protocolReason, "wechat: unsupported msg type")
	errEmptyContent           = apierror.BadRequest(protocolReason, "wechat: empty content")
	errAppCredentialsRequired = apierror.BadRequest(credentialReason, "wechat: app_id and app_secret required")
	errOpenIDRequired         = apierror.BadRequest(protocolReason, "wechat: open_id required")
)

func wechatAPIError(prefix string, msg string) error {
	return apierror.Internal(protocolReason, fmt.Sprintf("%s: %s", prefix, msg))
}

func wechatParseError(prefix string, err error) error {
	return apierror.Internal(protocolReason, fmt.Sprintf("%s: %v", prefix, err))
}
