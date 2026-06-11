package wecom

import (
	"fmt"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

const (
	configReason   = "WECOM_CONFIG"
	protocolReason = "WECOM_PROTOCOL"
)

var (
	errUnsupportedMsgType  = kerrors.BadRequest(protocolReason, "wecom: unsupported msgtype")
	errEmptyText           = kerrors.BadRequest(protocolReason, "wecom: empty text")
	errMissingTimestamp    = kerrors.BadRequest(protocolReason, "wecom: missing timestamp")
	errBadTimestamp        = kerrors.BadRequest(protocolReason, "wecom: bad timestamp")
	errTimestampOutOfRange = kerrors.BadRequest(protocolReason, "wecom: timestamp out of range")
	errBadSignature        = kerrors.BadRequest(protocolReason, "wecom: bad signature")
	errWebhookURLRequired  = kerrors.BadRequest(configReason, "wecom outbound: webhook url required")
	errResponseURLRequired = kerrors.BadRequest(configReason, "wecom: response_url required")
)

func wecomAPIError(prefix string, msg string) error {
	return kerrors.InternalServer(protocolReason, fmt.Sprintf("%s: %s", prefix, msg))
}

func wecomUnsupportedMsgTypeError(msgType string) error {
	return kerrors.BadRequest(protocolReason, fmt.Sprintf("wecom: unsupported msgtype %q", msgType))
}
