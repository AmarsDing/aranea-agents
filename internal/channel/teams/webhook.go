package teams

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"aranea-agents/internal/channel/port"
)

type InboundMessage struct {
	Text           string
	FromID         string
	FromName       string
	ChannelID      string
	ConversationID string
	ServiceURL     string
	ActivityID     string
	RecipientID    string
}

type activity struct {
	Type         string          `json:"type"`
	ID           string          `json:"id"`
	Timestamp    string          `json:"timestamp"`
	ServiceURL   string          `json:"serviceUrl"`
	ChannelID    string          `json:"channelId"`
	From         channelAccount  `json:"from"`
	Conversation conversation    `json:"conversation"`
	Recipient    channelAccount  `json:"recipient"`
	Text         string          `json:"text"`
	TextFormat   string          `json:"textFormat"`
	ChannelData  json.RawMessage `json:"channelData"`
}

type channelAccount struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type conversation struct {
	ID               string `json:"id"`
	ConversationType string `json:"conversationType"`
}

func ParseInbound(raw []byte) (InboundMessage, error) {
	var act activity
	if err := json.Unmarshal(raw, &act); err != nil {
		return InboundMessage{}, err
	}
	if strings.TrimSpace(strings.ToLower(act.Type)) != "message" {
		return InboundMessage{}, teamsUnsupportedActivityTypeError(act.Type)
	}
	text := strings.TrimSpace(act.Text)
	if text == "" {
		return InboundMessage{}, errEmptyText
	}
	return InboundMessage{
		Text:           text,
		FromID:         strings.TrimSpace(act.From.ID),
		FromName:       strings.TrimSpace(act.From.Name),
		ChannelID:      strings.TrimSpace(act.ChannelID),
		ConversationID: strings.TrimSpace(act.Conversation.ID),
		ServiceURL:     strings.TrimSpace(act.ServiceURL),
		ActivityID:     strings.TrimSpace(act.ID),
		RecipientID:    strings.TrimSpace(act.Recipient.ID),
	}, nil
}

// VerifyRequest validates a Bot Framework inbound Authorization bearer token.
// RS256 tokens are verified against Microsoft Bot Framework JWKS; HS256 uses appSecret.
func VerifyRequest(ctx context.Context, appID, appSecret string, header http.Header, body []byte) error {
	_ = body // reserved for future body-binding checks
	authHeader := strings.TrimSpace(header.Get("Authorization"))
	if authHeader == "" {
		return errMissingAuthHeader
	}
	if strings.TrimSpace(appID) == "" {
		return port.ErrCredentialsNotConfigured
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return verifyBotFrameworkToken(ctx, authHeader, appID, appSecret)
}

func verifyBotFrameworkToken(ctx context.Context, authHeader, appID, appSecret string) error {
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return errInvalidAuthScheme
	}
	token := strings.TrimSpace(parts[1])
	if token == "" {
		return errEmptyBearerToken
	}
	// Validate JWT structure (header.payload.signature)
	segments := strings.Split(token, ".")
	if len(segments) != 3 {
		return errInvalidJWTFormat
	}
	// Decode and validate header — reject "none" algorithm
	headerBytes, err := base64urlDecode(segments[0])
	if err != nil {
		return teamsJWTHeaderError(err)
	}
	var jwtHeader struct {
		Alg string `json:"alg"`
		Typ string `json:"typ"`
		Kid string `json:"kid"`
	}
	if err := json.Unmarshal(headerBytes, &jwtHeader); err != nil {
		return teamsJWTHeaderJSONError(err)
	}
	alg := strings.TrimSpace(strings.ToUpper(jwtHeader.Alg))
	if alg == "" || alg == "NONE" {
		return teamsAlgNotAllowedError(jwtHeader.Alg)
	}
	// Decode and validate payload claims
	payload, err := base64urlDecode(segments[1])
	if err != nil {
		return teamsJWTPayloadError(err)
	}
	var claims struct {
		Aud string  `json:"aud"`
		Iss string  `json:"iss"`
		Exp float64 `json:"exp"`
		Iat float64 `json:"iat"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return teamsJWTClaimsError(err)
	}
	// Check expiration
	now := time.Now().Unix()
	if int64(claims.Exp) < now-port.WebhookTimestampToleranceSec {
		return errTokenExpired
	}
	// Check audience matches appID
	if !strings.EqualFold(strings.TrimSpace(claims.Aud), strings.TrimSpace(appID)) {
		return errAudienceMismatch
	}
	// Verify signature based on algorithm
	switch alg {
	case "RS256":
		return verifyRS256(ctx, token, jwtHeader.Kid, appID, claims.Iss)
	case "HS256":
		if strings.TrimSpace(appSecret) == "" {
			return port.ErrCredentialsNotConfigured
		}
		// HMAC-SHA256 verification (used by some Bot Framework configurations)
		signingInput := segments[0] + "." + segments[1]
		key := deriveSigningKey(appID, appSecret)
		mac := hmac.New(sha256.New, key)
		mac.Write([]byte(signingInput))
		expectedSig := base64urlEncode(mac.Sum(nil))
		if !hmac.Equal([]byte(expectedSig), []byte(segments[2])) {
			return errSignatureFailed
		}
	default:
		return teamsUnsupportedJWTAlgError(jwtHeader.Alg)
	}
	return nil
}

func verifyRS256(ctx context.Context, token, kid, appID, iss string) error {
	if strings.TrimSpace(iss) != "" && !strings.EqualFold(strings.TrimSpace(iss), botFrameworkIssuer) {
		return errIssuerMismatch
	}
	pub, err := defaultJWKS.lookup(ctx, kid)
	if err != nil {
		return err
	}
	parser := jwt.NewParser(
		jwt.WithValidMethods([]string{jwt.SigningMethodRS256.Alg()}),
		jwt.WithAudience(strings.TrimSpace(appID)),
		jwt.WithIssuer(botFrameworkIssuer),
		jwt.WithLeeway(time.Duration(port.WebhookTimestampToleranceSec)*time.Second),
	)
	parsed, err := parser.Parse(token, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, errRS256VerifyFail
		}
		return pub, nil
	})
	if err != nil || parsed == nil || !parsed.Valid {
		return errRS256VerifyFail
	}
	return nil
}

func deriveSigningKey(_ string, appSecret string) []byte {
	// Microsoft Bot Framework uses the appSecret directly or base64-decoded
	secret := strings.TrimSpace(appSecret)
	if decoded, err := base64.StdEncoding.DecodeString(secret); err == nil && len(decoded) > 0 {
		return decoded
	}
	return []byte(secret)
}

func base64urlDecode(s string) ([]byte, error) {
	// Add padding if needed
	switch len(s) % 4 {
	case 2:
		s += "=="
	case 3:
		s += "="
	}
	return base64.URLEncoding.DecodeString(s)
}

func base64urlEncode(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}
