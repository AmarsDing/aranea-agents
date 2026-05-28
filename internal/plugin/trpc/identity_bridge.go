package plugintrpc

import (
	"context"

	trpcidentity "trpc.group/trpc-go/trpc-agent-go/plugin/identity"
)

type contextKey string

const (
	identityTokenKey contextKey = "aranea_identity_token"
)

func ContextWithToken(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, identityTokenKey, token)
}

func tokenFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(identityTokenKey).(string); ok {
		return v
	}
	return ""
}

type PlatformIdentityProvider struct {
	resolveAgent AgentKeyResolver
}

func NewPlatformIdentityProvider(resolveAgent AgentKeyResolver) *PlatformIdentityProvider {
	return &PlatformIdentityProvider{resolveAgent: resolveAgent}
}

func (p *PlatformIdentityProvider) Resolve(ctx context.Context, userID, sessionID string) (*trpcidentity.Identity, error) {
	id := &trpcidentity.Identity{
		UserID: userID,
	}
	if userID != "" {
		id.Headers = map[string]string{
			"X-User-ID": userID,
		}
		id.EnvVars = map[string]string{
			"ARANEA_USER_ID": userID,
		}
	}
	if sessionID != "" {
		if id.Headers == nil {
			id.Headers = map[string]string{}
		}
		id.Headers["X-Session-ID"] = sessionID
		if id.EnvVars == nil {
			id.EnvVars = map[string]string{}
		}
		id.EnvVars["ARANEA_SESSION_ID"] = sessionID
	}
	if token := tokenFromContext(ctx); token != "" {
		id.Token = token
		if id.Headers == nil {
			id.Headers = map[string]string{}
		}
		id.Headers["Authorization"] = "Bearer " + token
		if id.EnvVars == nil {
			id.EnvVars = map[string]string{}
		}
		id.EnvVars["ARANEA_ACCESS_TOKEN"] = token
	}
	return id, nil
}

func BuildIdentityPlugin(resolveAgent AgentKeyResolver) *trpcidentity.Plugin {
	provider := NewPlatformIdentityProvider(resolveAgent)
	return trpcidentity.NewPlugin(provider)
}
