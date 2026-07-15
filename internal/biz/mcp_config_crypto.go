package biz

import (
	"context"
	"encoding/json"
	"strings"

	"aranea-agents/pkg/apierror"
)

// mcpAPIRedactedPlaceholder matches internal/mcp/config.RedactConfigJSON output.
const mcpAPIRedactedPlaceholder = "******"

// ProcessMCPConfigJSONForStorage encrypts sensitive values in MCP config_json
// (api_key, token, client_secret, Authorization headers, …) to enc: refs (C-05).
// Already-encrypted enc: values and API redaction placeholders are left unchanged.
func (c *CredentialCrypto) ProcessMCPConfigJSONForStorage(ctx context.Context, cfg string) (string, error) {
	cfg = strings.TrimSpace(cfg)
	if cfg == "" || cfg == "{}" {
		return cfg, nil
	}
	var root any
	if err := json.Unmarshal([]byte(cfg), &root); err != nil {
		return "", apierror.BadRequest("MCP_SERVER", "config_json must be a valid JSON object")
	}
	out, err := c.walkEncryptMCPSecrets(ctx, "", root)
	if err != nil {
		return "", err
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return "", apierror.Internal("MCP_SERVER", "marshal encrypted mcp config failed: %s", err.Error())
	}
	return string(raw), nil
}

// DecryptMCPConfigJSONForRuntime expands enc: refs back to plaintext for probe/connect (C-05).
func (c *CredentialCrypto) DecryptMCPConfigJSONForRuntime(ctx context.Context, cfg string) (string, error) {
	cfg = strings.TrimSpace(cfg)
	if cfg == "" || cfg == "{}" {
		return cfg, nil
	}
	var root any
	if err := json.Unmarshal([]byte(cfg), &root); err != nil {
		return "", apierror.BadRequest("MCP_SERVER", "config_json must be a valid JSON object")
	}
	out, err := c.walkDecryptMCPSecrets(ctx, root)
	if err != nil {
		return "", err
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return "", apierror.Internal("MCP_SERVER", "marshal decrypted mcp config failed: %s", err.Error())
	}
	return string(raw), nil
}

// MergeMCPConfigJSONForUpdate deep-merges patch onto current, preserving encrypted
// secrets when the patch carries empty / redacted placeholders from API responses.
func MergeMCPConfigJSONForUpdate(current, patch string) (string, error) {
	current = strings.TrimSpace(current)
	patch = strings.TrimSpace(patch)
	if patch == "" {
		return current, nil
	}
	var pat any
	if err := json.Unmarshal([]byte(patch), &pat); err != nil {
		return "", apierror.BadRequest("MCP_SERVER", "config_json must be a valid JSON object")
	}
	var cur any
	if current != "" && current != "{}" {
		if err := json.Unmarshal([]byte(current), &cur); err != nil {
			return "", apierror.BadRequest("MCP_SERVER", "config_json must be a valid JSON object")
		}
	}
	merged := mergeMCPJSONValue(cur, pat)
	raw, err := json.Marshal(merged)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func (c *CredentialCrypto) walkEncryptMCPSecrets(ctx context.Context, parentKey string, v any) (any, error) {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, child := range t {
			enc, err := c.walkEncryptMCPSecrets(ctx, k, child)
			if err != nil {
				return nil, err
			}
			out[k] = enc
		}
		return out, nil
	case []any:
		out := make([]any, len(t))
		for i, child := range t {
			enc, err := c.walkEncryptMCPSecrets(ctx, parentKey, child)
			if err != nil {
				return nil, err
			}
			out[i] = enc
		}
		return out, nil
	case string:
		if !isMCPSensitiveConfigKey(parentKey) {
			return t, nil
		}
		plain := strings.TrimSpace(t)
		if plain == "" || plain == mcpAPIRedactedPlaceholder {
			return t, nil
		}
		if strings.HasPrefix(plain, channelSecretRefPrefix) {
			return plain, nil
		}
		ref, err := c.EncryptChannelSecretRef(ctx, plain)
		if err != nil {
			return nil, err
		}
		return ref, nil
	default:
		return v, nil
	}
}

func (c *CredentialCrypto) walkDecryptMCPSecrets(ctx context.Context, v any) (any, error) {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, child := range t {
			dec, err := c.walkDecryptMCPSecrets(ctx, child)
			if err != nil {
				return nil, err
			}
			out[k] = dec
		}
		return out, nil
	case []any:
		out := make([]any, len(t))
		for i, child := range t {
			dec, err := c.walkDecryptMCPSecrets(ctx, child)
			if err != nil {
				return nil, err
			}
			out[i] = dec
		}
		return out, nil
	case string:
		if !strings.HasPrefix(strings.TrimSpace(t), channelSecretRefPrefix) {
			return t, nil
		}
		plain, err := c.DecryptChannelSecretRef(ctx, t)
		if err != nil {
			return nil, err
		}
		return plain, nil
	default:
		return v, nil
	}
}

func mergeMCPJSONValue(cur, pat any) any {
	curMap, curOK := cur.(map[string]any)
	patMap, patOK := pat.(map[string]any)
	if curOK && patOK {
		out := make(map[string]any, len(curMap)+len(patMap))
		for k, v := range curMap {
			out[k] = v
		}
		for k, pv := range patMap {
			cv, has := curMap[k]
			if s, ok := pv.(string); ok && isMCPSensitiveConfigKey(k) && shouldPreserveMCPSecret(s) && has {
				out[k] = cv
				continue
			}
			if has {
				out[k] = mergeMCPJSONValue(cv, pv)
			} else {
				out[k] = pv
			}
		}
		return out
	}
	return pat
}

func shouldPreserveMCPSecret(v string) bool {
	v = strings.TrimSpace(v)
	return v == "" || v == mcpAPIRedactedPlaceholder
}

func isMCPSensitiveConfigKey(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	// Exact / suffix matches for known secret field names.
	switch key {
	case "api_key", "apikey", "client_secret", "access_token", "refresh_token",
		"authorization", "password", "secret", "token", "cookie", "bearer":
		return true
	}
	// Nested header / env keys that embed secret tokens (but not token_url).
	if strings.HasSuffix(key, "_secret") || strings.HasSuffix(key, "_password") {
		return true
	}
	if strings.HasSuffix(key, "_token") || strings.HasSuffix(key, "_key") {
		return true
	}
	if strings.Contains(key, "authorization") || strings.Contains(key, "password") {
		return true
	}
	if key == "api-key" || key == "x-api-key" {
		return true
	}
	return false
}
