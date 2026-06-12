package teams

import (
	"fmt"

	"aranea-agents/pkg/apierror"
)

const (
	configReason     = "TEAMS_CONFIG"
	credentialReason = "TEAMS_CREDENTIAL"
	protocolReason   = "TEAMS_PROTOCOL"
)

var (
	errUnsupportedActivityType = apierror.BadRequest(protocolReason, "teams: unsupported activity type")
	errEmptyText               = apierror.BadRequest(protocolReason, "teams: empty text")
	errMissingAuthHeader       = apierror.BadRequest(protocolReason, "teams: missing authorization header")
	errInvalidAuthScheme       = apierror.BadRequest(protocolReason, "teams: invalid authorization scheme")
	errEmptyBearerToken        = apierror.BadRequest(protocolReason, "teams: empty bearer token")
	errInvalidJWTFormat        = apierror.BadRequest(protocolReason, "teams: invalid JWT format")
	errInvalidJWTHeader        = apierror.BadRequest(protocolReason, "teams: invalid JWT header")
	errInvalidJWTHeaderJSON    = apierror.BadRequest(protocolReason, "teams: invalid JWT header JSON")
	errJWTAlgNotAllowed        = apierror.BadRequest(protocolReason, "teams: JWT algorithm not allowed")
	errInvalidJWTPayload       = apierror.BadRequest(protocolReason, "teams: invalid JWT payload")
	errInvalidJWTClaims        = apierror.BadRequest(protocolReason, "teams: invalid JWT claims")
	errTokenExpired            = apierror.BadRequest(protocolReason, "teams: token expired")
	errAudienceMismatch        = apierror.BadRequest(protocolReason, "teams: audience mismatch")
	errRS256NotImplemented     = apierror.BadRequest(protocolReason, "teams: RS256 verification not yet implemented, use HS256 or configure app secret")
	errSignatureFailed         = apierror.BadRequest(protocolReason, "teams: signature verification failed")
	errUnsupportedJWTAlg       = apierror.BadRequest(protocolReason, "teams: unsupported JWT algorithm")
	errAppCredentialsRequired  = apierror.BadRequest(credentialReason, "teams: app_id and app_secret required")
	errRecipientRequired       = apierror.BadRequest(protocolReason, "teams: recipient required")
	errServiceURLRequired      = apierror.BadRequest(configReason, "teams: service_url required")
	errConversationIDRequired  = apierror.BadRequest(protocolReason, "teams: conversation_id required")
)

func teamsAPIError(prefix string, msg string) error {
	return apierror.Internal(protocolReason, fmt.Sprintf("%s: %s", prefix, msg))
}

func teamsJWTHeaderError(err error) error {
	return apierror.BadRequest(protocolReason, fmt.Sprintf("teams: invalid JWT header: %v", err))
}

func teamsJWTHeaderJSONError(err error) error {
	return apierror.BadRequest(protocolReason, fmt.Sprintf("teams: invalid JWT header JSON: %v", err))
}

func teamsJWTPayloadError(err error) error {
	return apierror.BadRequest(protocolReason, fmt.Sprintf("teams: invalid JWT payload: %v", err))
}

func teamsJWTClaimsError(err error) error {
	return apierror.BadRequest(protocolReason, fmt.Sprintf("teams: invalid JWT claims: %v", err))
}

func teamsAlgNotAllowedError(alg string) error {
	return apierror.BadRequest(protocolReason, fmt.Sprintf("teams: JWT algorithm %q not allowed", alg))
}

func teamsUnsupportedActivityTypeError(actType string) error {
	return apierror.BadRequest(protocolReason, fmt.Sprintf("teams: unsupported activity type %q", actType))
}

func teamsUnsupportedJWTAlgError(alg string) error {
	return apierror.BadRequest(protocolReason, fmt.Sprintf("teams: unsupported JWT algorithm %q", alg))
}
