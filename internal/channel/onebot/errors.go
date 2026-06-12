package onebot

import (
	"fmt"

	"aranea-agents/pkg/apierror"
)

const (
	configReason   = "ONEBOT_CONFIG"
	protocolReason = "ONEBOT_PROTOCOL"
)

var (
	errHTTPServerRequired = apierror.BadRequest(configReason, "onebot outbound: http_server required")
	errBadSignature       = apierror.BadRequest(protocolReason, "onebot: bad signature")
	errEmptyMessage       = apierror.BadRequest(protocolReason, "onebot: empty message")
	errRecipientRequired  = apierror.BadRequest(protocolReason, "onebot: recipient required")
)

func onebotAPIError(prefix string, msg string) error {
	return apierror.Internal(protocolReason, fmt.Sprintf("%s: %s", prefix, msg))
}
