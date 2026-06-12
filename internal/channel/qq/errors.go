package qq

import (
	"fmt"

	"aranea-agents/pkg/apierror"
)

const (
	configReason   = "QQ_CONFIG"
	protocolReason = "QQ_PROTOCOL"
)

var (
	errBadSignature             = apierror.BadRequest(protocolReason, "qq: bad signature")
	errInvalidValidationPayload = apierror.BadRequest(protocolReason, "qq: invalid validation payload")
	errMissingValidationFields  = apierror.BadRequest(protocolReason, "qq: missing validation fields")
	errUnsupportedEvent         = apierror.BadRequest(protocolReason, "qq: unsupported event")
	errEmptyContent             = apierror.BadRequest(protocolReason, "qq: empty content")
	errAppCredentialsRequired   = apierror.BadRequest(configReason, "qq outbound: app_id and app_secret required")
	errRecipientRequired        = apierror.BadRequest(protocolReason, "qq: recipient required")
)

func qqUnsupportedEventError(eventType string) error {
	return apierror.BadRequest(protocolReason, fmt.Sprintf("qq: unsupported event %s", eventType))
}
