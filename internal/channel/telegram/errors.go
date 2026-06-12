package telegram

import (
	"fmt"

	"aranea-agents/pkg/apierror"
)

const (
	configReason     = "TELEGRAM_CONFIG"
	credentialReason = "TELEGRAM_CREDENTIAL"
	protocolReason   = "TELEGRAM_PROTOCOL"
)

var (
	errBotTokenRequired    = apierror.BadRequest(configReason, "telegram: bot_token required")
	errBadChatID           = apierror.BadRequest(protocolReason, "telegram: bad chat id")
	errNoMessage           = apierror.BadRequest(protocolReason, "telegram: no message")
	errBotMessageIgnored   = apierror.BadRequest(protocolReason, "telegram: bot message ignored")
	errEmptyText           = apierror.BadRequest(protocolReason, "telegram: empty text")
	errMissingChatID       = apierror.BadRequest(protocolReason, "telegram: missing chat id")
	errSecretTokenMismatch = apierror.BadRequest(credentialReason, "telegram: secret token mismatch")
)

func telegramAPIError(prefix string, msg string) error {
	return apierror.Internal(protocolReason, fmt.Sprintf("%s: %s", prefix, msg))
}

func telegramParseError(prefix string, err error) error {
	return apierror.Internal(protocolReason, fmt.Sprintf("%s: %v", prefix, err))
}
