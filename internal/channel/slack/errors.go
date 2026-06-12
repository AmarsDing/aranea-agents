package slack

import (
	"fmt"

	"aranea-agents/pkg/apierror"
)

const (
	configReason     = "SLACK_CONFIG"
	credentialReason = "SLACK_CREDENTIAL"
	protocolReason   = "SLACK_PROTOCOL"
)

var (
	errBotTokenRequired       = apierror.BadRequest(configReason, "slack: bot_token required")
	errBotTokenAndAppRequired = apierror.BadRequest(configReason, "slack socket_mode: bot_token and app_token required")
	errEmptyChallenge         = apierror.BadRequest(protocolReason, "slack: empty challenge")
	errUnsupportedEventType   = apierror.BadRequest(protocolReason, "slack: unsupported event type")
	errIgnoredMessageSubtype  = apierror.BadRequest(protocolReason, "slack: ignored message subtype")
	errEmptyMessage           = apierror.BadRequest(protocolReason, "slack: empty message")
	errUnsupportedPayloadType = apierror.BadRequest(protocolReason, "slack: unsupported payload type")
	errMissingSignature       = apierror.BadRequest(protocolReason, "slack: missing signature headers")
	errBadTimestamp           = apierror.BadRequest(protocolReason, "slack: bad timestamp")
	errTimestampOutOfRange    = apierror.BadRequest(protocolReason, "slack: timestamp out of range")
	errBadSignature           = apierror.BadRequest(protocolReason, "slack: bad signature")
	errStreamChannelRequired  = apierror.BadRequest(configReason, "slack stream: channel required")
	errStreamBotTokenRequired = apierror.BadRequest(configReason, "slack stream: bot_token required")
)

func slackAPIError(prefix string, msg string) error {
	return apierror.Internal(protocolReason, fmt.Sprintf("%s: %s", prefix, msg))
}

func slackParseError(prefix string, err error) error {
	return apierror.Internal(protocolReason, fmt.Sprintf("%s: %v", prefix, err))
}
