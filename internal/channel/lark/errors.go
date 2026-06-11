package lark

// Package lark centralises IM-platform error sentinels for the Feishu / Lark
// adapter. The rest of the codebase should treat the symbols in this file as
// the canonical error vocabulary for lark-side failures.
//
// All exported sentinels are constructed with the project-standard kerrors
// helpers so that callers can rely on `kerrors.Is*` checks at the boundary
// (Service / biz layer) and on `kerrors.FromError` for HTTP status mapping.
//
// The errors fall into three categories:
//
//   - **Config** — the channel row or credential table is missing required
//     fields. BadRequest (HTTP 400) is the natural mapping; channel operators
//     fix these by editing the channel config, not by retrying.
//
//   - **Credential** — the credential was looked up but resolved to an empty
//     value (missing key, decryption failure surfaced as empty). The Service
//     layer treats these as a configuration bug and surfaces them as
//     InternalServer (HTTP 500) so alerts fire.
//
//   - **Protocol** — the Feishu API itself returned a non-zero code, the
//     webhook signature did not validate, or the encrypted payload could not
//     be decrypted. BadRequest maps naturally and the Kratos middleware
//     already logs the upstream response code at the WARN level.
import (
	"fmt"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

const (
	configReason     = "LARK_CONFIG"
	credentialReason = "LARK_CREDENTIAL"
	protocolReason   = "LARK_PROTOCOL"
)

// errConfigJSONMalformed is returned when channel config_json cannot be parsed.
var errConfigJSONMalformed = kerrors.BadRequest(configReason, "config_json is not valid JSON")

// errAppIDRequired is returned when the Feishu app_id is missing or empty.
var errAppIDRequired = kerrors.BadRequest(configReason, "config_json.config.app_id is required")

// errAppSecretRequired is returned when the channel has no usable app_secret.
// Distinct from errConfigJSONMalformed because it usually points at a missing
// credential row, not a malformed one.
var errAppSecretRequired = kerrors.BadRequest(credentialReason, "app_secret is required")

// errSignatureMissing is returned by the webhook handler when the required
// signature headers are absent on the request.
var errSignatureMissing = kerrors.BadRequest(protocolReason, "missing signature headers")

// errSignatureMismatch is returned by the webhook handler when the signature
// did not validate against the channel's verification token / encrypt_key.
var errSignatureMismatch = kerrors.BadRequest(protocolReason, "signature mismatch")

// errVerificationTokenMismatch is returned by the URL-verification handshake
// when the inbound token does not match the channel's verification_token.
var errVerificationTokenMismatch = kerrors.BadRequest(protocolReason, "verification token mismatch")

// errEncryptKeyMissing is returned when an incoming payload is encrypted but
// the channel has no encrypt_key configured. The adapter cannot decrypt
// without it; the operator must add the credential and reload.
var errEncryptKeyMissing = kerrors.BadRequest(configReason, "encrypted payload but encrypt_key is missing")

// errDecryptEmptyKeyOrCipher is the lowest-level decrypt failure guard.
var errDecryptEmptyKeyOrCipher = kerrors.BadRequest(protocolReason, "empty decrypt key or cipher")

// errDecryptInvalidPadding is returned by the AES-CBC pad-unpad code path
// when the trailing bytes are not a valid PKCS#7 padding block.
var errDecryptInvalidPadding = kerrors.BadRequest(protocolReason, "invalid padding")

// errDecryptInvalidBlockSize is returned when the ciphertext length is not a
// multiple of the AES block size.
var errDecryptInvalidBlockSize = kerrors.BadRequest(protocolReason, "invalid block size")

// errReceiveIDRequired guards message-sending paths (interactive card,
// stream send, plain send) against a common caller mistake.
var errReceiveIDRequired = kerrors.BadRequest(protocolReason, "receive_id is required")

// errCardOrReceiveIDRequired guards the interactive card send path.
var errCardOrReceiveIDRequired = kerrors.BadRequest(protocolReason, "receive_id and card are required")

// errMessageIDAndCardRequired guards the interactive card update path.
var errMessageIDAndCardRequired = kerrors.BadRequest(protocolReason, "message_id and card are required")

// errReceiveIDAndTextRequired guards the plain-text send path.
var errReceiveIDAndTextRequired = kerrors.BadRequest(protocolReason, "receive_id and text are required")

// errEmptyMessageID is returned by the streaming send path when the upstream
// returned a 0-code response but no message_id, which usually means the
// message was suppressed (e.g. tenant mute) and we should treat it as a
// "did not deliver" signal rather than a success.
var errEmptyMessageID = kerrors.BadRequest(protocolReason, "upstream returned empty message_id")

// errEmptyCardActionValue is returned by the card action handler when the
// inbound action payload has no `value` field. We cannot dispatch the action
// without it, but it is a client (card builder) error, not a server error.
var errEmptyCardActionValue = kerrors.BadRequest(protocolReason, "empty card action value")

// errMissingCardActionResult is returned by the card action handler when
// the action's `value` field is missing the expected `result` key.
var errMissingCardActionResult = kerrors.BadRequest(protocolReason, "missing card action result")

// errEmptyTenantToken is returned by the token-fetch path when the Feishu
// API returned a 0-code response but no token. Same shape as errEmptyMessageID.
var errEmptyTenantToken = kerrors.BadRequest(protocolReason, "upstream returned empty tenant_access_token")

// errNilRequest is returned when an HTTP request is nil.
var errNilRequest = kerrors.BadRequest(protocolReason, "nil request")

// errBadTimestamp is returned when the webhook timestamp cannot be parsed.
var errBadTimestamp = kerrors.BadRequest(protocolReason, "bad timestamp format")

// errTimestampOutOfRange is returned when the webhook timestamp is outside the tolerance window.
var errTimestampOutOfRange = kerrors.BadRequest(protocolReason, "timestamp out of range")

// errDecryptBase64 is returned when the base64 decode of the cipher text fails.
var errDecryptBase64 = kerrors.BadRequest(protocolReason, "base64 decode failed")

// errCipherTooShort is returned when the cipher text is shorter than AES block size.
var errCipherTooShort = kerrors.BadRequest(protocolReason, "cipher text too short")

// errAppCredentialsRequired is returned when app_id or app_secret is missing for API calls.
var errAppCredentialsRequired = kerrors.BadRequest(credentialReason, "app_id and app_secret required")

// feishuAPIError creates a kerrors.InternalServer for Feishu API non-zero responses.
// Used when the upstream Feishu API returns a non-zero code.
func feishuAPIError(prefix string, code int, msg string) error {
	return kerrors.InternalServer(protocolReason, fmt.Sprintf("%s: code=%d msg=%s", prefix, code, msg))
}

// feishuParseError creates a kerrors.InternalServer for Feishu API response parse failures.
// Used when the upstream Feishu API returns a response that cannot be parsed.
func feishuParseError(prefix string, err error) error {
	return kerrors.InternalServer(protocolReason, fmt.Sprintf("%s: %v", prefix, err))
}

// feishuInboundParseError creates a kerrors.BadRequest for inbound webhook parse failures.
// Used when the inbound request body cannot be parsed — this is a client error (400),
// not a server error, because the sender submitted malformed JSON.
func feishuInboundParseError(prefix string, err error) error {
	return kerrors.BadRequest(protocolReason, fmt.Sprintf("%s: %v", prefix, err))
}
