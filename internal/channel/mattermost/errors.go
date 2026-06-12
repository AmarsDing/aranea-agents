package mattermost

import (
	"fmt"

	"aranea-agents/pkg/apierror"
)

const (
	configReason   = "MATTERMOST_CONFIG"
	protocolReason = "MATTERMOST_PROTOCOL"
)

var (
	errServerURLAndBotTokenRequired = apierror.BadRequest(configReason, "mattermost: server_url and bot_token required")
	errBotTokenRequired             = apierror.BadRequest(configReason, "mattermost: bot_token required")
	errServerURLRequired            = apierror.BadRequest(configReason, "mattermost: server_url required")
	errEmptyText                    = apierror.BadRequest(protocolReason, "mattermost: empty text")
	errBadToken                     = apierror.BadRequest(protocolReason, "mattermost: bad token")
	errMissingSignature             = apierror.BadRequest(protocolReason, "mattermost: missing signature")
	errBadSignature                 = apierror.BadRequest(protocolReason, "mattermost: bad signature")
	errEmptyPostID                  = apierror.BadRequest(protocolReason, "mattermost: empty post id")
	errChannelIDRequired            = apierror.BadRequest(protocolReason, "mattermost: channel_id required")
)

func mattermostAPIError(prefix string, msg string) error {
	return apierror.Internal(protocolReason, fmt.Sprintf("%s: %s", prefix, msg))
}

func mattermostParseError(prefix string, err error) error {
	return apierror.Internal(protocolReason, fmt.Sprintf("%s: %v", prefix, err))
}
