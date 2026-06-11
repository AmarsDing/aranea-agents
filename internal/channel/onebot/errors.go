package onebot

import (
	"fmt"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

const (
	configReason   = "ONEBOT_CONFIG"
	protocolReason = "ONEBOT_PROTOCOL"
)

var (
	errHTTPServerRequired = kerrors.BadRequest(configReason, "onebot outbound: http_server required")
	errBadSignature       = kerrors.BadRequest(protocolReason, "onebot: bad signature")
	errEmptyMessage       = kerrors.BadRequest(protocolReason, "onebot: empty message")
	errRecipientRequired  = kerrors.BadRequest(protocolReason, "onebot: recipient required")
)

func onebotAPIError(prefix string, msg string) error {
	return kerrors.InternalServer(protocolReason, fmt.Sprintf("%s: %s", prefix, msg))
}
