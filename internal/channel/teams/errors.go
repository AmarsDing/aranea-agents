package teams

import (
	"fmt"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

const (
	configReason     = "TEAMS_CONFIG"
	credentialReason = "TEAMS_CREDENTIAL"
	protocolReason   = "TEAMS_PROTOCOL"
)

var (
	errUnsupportedActivityType = kerrors.BadRequest(protocolReason, "teams: unsupported activity type")
	errEmptyText               = kerrors.BadRequest(protocolReason, "teams: empty text")
	errMissingAuthHeader       = kerrors.BadRequest(protocolReason, "teams: missing authorization header")
	errInvalidAuthScheme       = kerrors.BadRequest(protocolReason, "teams: invalid authorization scheme")
	errEmptyBearerToken        = kerrors.BadRequest(protocolReason, "teams: empty bearer token")
	errInvalidJWTFormat        = kerrors.BadRequest(protocolReason, "teams: invalid JWT format")
	errInvalidJWTHeader        = kerrors.BadRequest(protocolReason, "teams: invalid JWT header")
	errInvalidJWTHeaderJSON    = kerrors.BadRequest(protocolReason, "teams: invalid JWT header JSON")
	errJWTAlgNotAllowed        = kerrors.BadRequest(protocolReason, "teams: JWT algorithm not allowed")
	errInvalidJWTPayload       = kerrors.BadRequest(protocolReason, "teams: invalid JWT payload")
	errInvalidJWTClaims        = kerrors.BadRequest(protocolReason, "teams: invalid JWT claims")
	errTokenExpired            = kerrors.BadRequest(protocolReason, "teams: token expired")
	errAudienceMismatch        = kerrors.BadRequest(protocolReason, "teams: audience mismatch")
	errRS256NotImplemented     = kerrors.BadRequest(protocolReason, "teams: RS256 verification not yet implemented, use HS256 or configure app secret")
	errSignatureFailed         = kerrors.BadRequest(protocolReason, "teams: signature verification failed")
	errUnsupportedJWTAlg       = kerrors.BadRequest(protocolReason, "teams: unsupported JWT algorithm")
	errAppCredentialsRequired  = kerrors.BadRequest(credentialReason, "teams: app_id and app_secret required")
	errRecipientRequired       = kerrors.BadRequest(protocolReason, "teams: recipient required")
	errServiceURLRequired      = kerrors.BadRequest(configReason, "teams: service_url required")
	errConversationIDRequired  = kerrors.BadRequest(protocolReason, "teams: conversation_id required")
)

func teamsAPIError(prefix string, msg string) error {
	return kerrors.InternalServer(protocolReason, fmt.Sprintf("%s: %s", prefix, msg))
}

func teamsJWTHeaderError(err error) error {
	return kerrors.BadRequest(protocolReason, fmt.Sprintf("teams: invalid JWT header: %v", err))
}

func teamsJWTHeaderJSONError(err error) error {
	return kerrors.BadRequest(protocolReason, fmt.Sprintf("teams: invalid JWT header JSON: %v", err))
}

func teamsJWTPayloadError(err error) error {
	return kerrors.BadRequest(protocolReason, fmt.Sprintf("teams: invalid JWT payload: %v", err))
}

func teamsJWTClaimsError(err error) error {
	return kerrors.BadRequest(protocolReason, fmt.Sprintf("teams: invalid JWT claims: %v", err))
}

func teamsAlgNotAllowedError(alg string) error {
	return kerrors.BadRequest(protocolReason, fmt.Sprintf("teams: JWT algorithm %q not allowed", alg))
}

func teamsUnsupportedActivityTypeError(actType string) error {
	return kerrors.BadRequest(protocolReason, fmt.Sprintf("teams: unsupported activity type %q", actType))
}

func teamsUnsupportedJWTAlgError(alg string) error {
	return kerrors.BadRequest(protocolReason, fmt.Sprintf("teams: unsupported JWT algorithm %q", alg))
}
