package telegram

import (
	"fmt"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

const (
	configReason     = "TELEGRAM_CONFIG"
	credentialReason = "TELEGRAM_CREDENTIAL"
	protocolReason   = "TELEGRAM_PROTOCOL"
)

var (
	errBotTokenRequired    = kerrors.BadRequest(configReason, "telegram: bot_token required")
	errBadChatID           = kerrors.BadRequest(protocolReason, "telegram: bad chat id")
	errNoMessage           = kerrors.BadRequest(protocolReason, "telegram: no message")
	errBotMessageIgnored   = kerrors.BadRequest(protocolReason, "telegram: bot message ignored")
	errEmptyText           = kerrors.BadRequest(protocolReason, "telegram: empty text")
	errMissingChatID       = kerrors.BadRequest(protocolReason, "telegram: missing chat id")
	errSecretTokenMismatch = kerrors.BadRequest(credentialReason, "telegram: secret token mismatch")
)

func telegramAPIError(prefix string, msg string) error {
	return kerrors.InternalServer(protocolReason, fmt.Sprintf("%s: %s", prefix, msg))
}

func telegramParseError(prefix string, err error) error {
	return kerrors.InternalServer(protocolReason, fmt.Sprintf("%s: %v", prefix, err))
}
