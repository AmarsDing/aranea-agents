package discord

import (
	"fmt"

	"aranea-agents/pkg/apierror"
)

const (
	configReason   = "DISCORD_CONFIG"
	protocolReason = "DISCORD_PROTOCOL"
)

var (
	errBotTokenRequired  = apierror.BadRequest(configReason, "discord: bot_token required")
	errChannelIDRequired = apierror.BadRequest(protocolReason, "discord: channel_id required")
)

func discordAPIError(prefix string, msg string) error {
	return apierror.Internal(protocolReason, fmt.Sprintf("%s: %s", prefix, msg))
}

func discordGatewayError(prefix string, err error) error {
	return apierror.Internal(protocolReason, fmt.Sprintf("%s: %v", prefix, err))
}
