package wecom

import (
	"fmt"

	"aranea-agents/pkg/apierror"
)

const (
	configReason   = "WECOM_CONFIG"
	protocolReason = "WECOM_PROTOCOL"
)

var (
	errUnsupportedMsgType  = apierror.BadRequest(protocolReason, "wecom: unsupported msgtype")
	errEmptyText           = apierror.BadRequest(protocolReason, "wecom: empty text")
	errMissingTimestamp    = apierror.BadRequest(protocolReason, "wecom: missing timestamp")
	errBadTimestamp        = apierror.BadRequest(protocolReason, "wecom: bad timestamp")
	errTimestampOutOfRange = apierror.BadRequest(protocolReason, "wecom: timestamp out of range")
	errBadSignature        = apierror.BadRequest(protocolReason, "wecom: bad signature")
	errWebhookURLRequired  = apierror.BadRequest(configReason, "wecom outbound: webhook url required")
	errResponseURLRequired = apierror.BadRequest(configReason, "wecom: response_url required")
)

func wecomAPIError(prefix string, msg string) error {
	return apierror.Internal(protocolReason, fmt.Sprintf("%s: %s", prefix, msg))
}

func wecomUnsupportedMsgTypeError(msgType string) error {
	return apierror.BadRequest(protocolReason, fmt.Sprintf("wecom: unsupported msgtype %q", msgType))
}
