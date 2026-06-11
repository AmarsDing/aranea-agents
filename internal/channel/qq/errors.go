package qq

import (
	"fmt"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

const (
	configReason   = "QQ_CONFIG"
	protocolReason = "QQ_PROTOCOL"
)

var (
	errBadSignature             = kerrors.BadRequest(protocolReason, "qq: bad signature")
	errInvalidValidationPayload = kerrors.BadRequest(protocolReason, "qq: invalid validation payload")
	errMissingValidationFields  = kerrors.BadRequest(protocolReason, "qq: missing validation fields")
	errUnsupportedEvent         = kerrors.BadRequest(protocolReason, "qq: unsupported event")
	errEmptyContent             = kerrors.BadRequest(protocolReason, "qq: empty content")
	errAppCredentialsRequired   = kerrors.BadRequest(configReason, "qq outbound: app_id and app_secret required")
	errRecipientRequired        = kerrors.BadRequest(protocolReason, "qq: recipient required")
)

func qqUnsupportedEventError(eventType string) error {
	return kerrors.BadRequest(protocolReason, fmt.Sprintf("qq: unsupported event %s", eventType))
}
