package discord

import (
	"fmt"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

const (
	configReason   = "DISCORD_CONFIG"
	protocolReason = "DISCORD_PROTOCOL"
)

var (
	errBotTokenRequired  = kerrors.BadRequest(configReason, "discord: bot_token required")
	errChannelIDRequired = kerrors.BadRequest(protocolReason, "discord: channel_id required")
)

func discordAPIError(prefix string, msg string) error {
	return kerrors.InternalServer(protocolReason, fmt.Sprintf("%s: %s", prefix, msg))
}

func discordGatewayError(prefix string, err error) error {
	return kerrors.InternalServer(protocolReason, fmt.Sprintf("%s: %v", prefix, err))
}
