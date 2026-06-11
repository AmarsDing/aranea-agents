package mattermost

import (
	"fmt"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

const (
	configReason   = "MATTERMOST_CONFIG"
	protocolReason = "MATTERMOST_PROTOCOL"
)

var (
	errServerURLAndBotTokenRequired = kerrors.BadRequest(configReason, "mattermost: server_url and bot_token required")
	errBotTokenRequired             = kerrors.BadRequest(configReason, "mattermost: bot_token required")
	errServerURLRequired            = kerrors.BadRequest(configReason, "mattermost: server_url required")
	errEmptyText                    = kerrors.BadRequest(protocolReason, "mattermost: empty text")
	errBadToken                     = kerrors.BadRequest(protocolReason, "mattermost: bad token")
	errMissingSignature             = kerrors.BadRequest(protocolReason, "mattermost: missing signature")
	errBadSignature                 = kerrors.BadRequest(protocolReason, "mattermost: bad signature")
	errEmptyPostID                  = kerrors.BadRequest(protocolReason, "mattermost: empty post id")
	errChannelIDRequired            = kerrors.BadRequest(protocolReason, "mattermost: channel_id required")
)

func mattermostAPIError(prefix string, msg string) error {
	return kerrors.InternalServer(protocolReason, fmt.Sprintf("%s: %s", prefix, msg))
}

func mattermostParseError(prefix string, err error) error {
	return kerrors.InternalServer(protocolReason, fmt.Sprintf("%s: %v", prefix, err))
}
