package slack

import (
	"fmt"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

const (
	configReason     = "SLACK_CONFIG"
	credentialReason = "SLACK_CREDENTIAL"
	protocolReason   = "SLACK_PROTOCOL"
)

var (
	errBotTokenRequired       = kerrors.BadRequest(configReason, "slack: bot_token required")
	errBotTokenAndAppRequired = kerrors.BadRequest(configReason, "slack socket_mode: bot_token and app_token required")
	errEmptyChallenge         = kerrors.BadRequest(protocolReason, "slack: empty challenge")
	errUnsupportedEventType   = kerrors.BadRequest(protocolReason, "slack: unsupported event type")
	errIgnoredMessageSubtype  = kerrors.BadRequest(protocolReason, "slack: ignored message subtype")
	errEmptyMessage           = kerrors.BadRequest(protocolReason, "slack: empty message")
	errUnsupportedPayloadType = kerrors.BadRequest(protocolReason, "slack: unsupported payload type")
	errMissingSignature       = kerrors.BadRequest(protocolReason, "slack: missing signature headers")
	errBadTimestamp           = kerrors.BadRequest(protocolReason, "slack: bad timestamp")
	errTimestampOutOfRange    = kerrors.BadRequest(protocolReason, "slack: timestamp out of range")
	errBadSignature           = kerrors.BadRequest(protocolReason, "slack: bad signature")
	errStreamChannelRequired  = kerrors.BadRequest(configReason, "slack stream: channel required")
	errStreamBotTokenRequired = kerrors.BadRequest(configReason, "slack stream: bot_token required")
)

func slackAPIError(prefix string, msg string) error {
	return kerrors.InternalServer(protocolReason, fmt.Sprintf("%s: %s", prefix, msg))
}

func slackParseError(prefix string, err error) error {
	return kerrors.InternalServer(protocolReason, fmt.Sprintf("%s: %v", prefix, err))
}
