package biz

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"strings"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

const envCredentialKey = "ARANEA_CREDENTIAL_KEY"

const credentialKeyRequiredMsg = "credential encryption key is required (configure system settings or ARANEA_CREDENTIAL_KEY)"

const invalidProviderConfigJSONMsg = "config_json must be valid JSON object"

func parseProviderConfigJSON(cfg string) (map[string]any, error) {
	cfg = strings.TrimSpace(cfg)
	if cfg == "" {
		return nil, nil
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(cfg), &m); err != nil || m == nil {
		return nil, apierror.BadRequest("LLM_PROVIDER_MODEL", invalidProviderConfigJSONMsg)
	}
	return m, nil
}

func (c *CredentialCrypto) encryptCredential(ctx context.Context, plain string) (string, error) {
	plain = strings.TrimSpace(plain)
	if plain == "" {
		return "", nil
	}
	key, err := c.aesKey(ctx)
	if err != nil {
		return "", err
	}
	if len(key) != 32 {
		return "", nil
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plain), nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

func (c *CredentialCrypto) decryptCredential(ctx context.Context, enc string) (string, error) {
	enc = strings.TrimSpace(enc)
	if enc == "" {
		return "", nil
	}
	key, err := c.aesKey(ctx)
	if err != nil {
		return "", err
	}
	if len(key) != 32 {
		return enc, nil
	}
	raw, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonceSize := gcm.NonceSize()
	if len(raw) < nonceSize {
		return "", err
	}
	plain, err := gcm.Open(nil, raw[:nonceSize], raw[nonceSize:], nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

type ProviderCredentialsReveal struct {
	APIKey       string
	SecretKey    string
	HasAPIKey    bool
	HasSecretKey bool
	HACandidates []HACandidateCredentialReveal
}

type HACandidateCredentialReveal struct {
	Name   string
	APIKey string
}

func mergeConfigJSONForUpdate(current, patch string) (string, error) {
	current = strings.TrimSpace(current)
	patch = strings.TrimSpace(patch)
	if patch == "" {
		return current, nil
	}
	pat, err := parseProviderConfigJSON(patch)
	if err != nil {
		return "", err
	}
	var cur map[string]any
	if strings.TrimSpace(current) != "" {
		cur, err = parseProviderConfigJSON(current)
		if err != nil {
			return "", err
		}
	}
	if cur == nil {
		cur = map[string]any{}
	}
	if !hasNonEmptyString(pat, "api_key") {
		copySecretField(cur, pat, "api_key_enc")
		if v, ok := cur["api_key_set"]; ok {
			pat["api_key_set"] = v
		}
		delete(pat, "api_key")
	}
	if !hasNonEmptyString(pat, "secret_key") {
		copySecretField(cur, pat, "secret_key_enc")
		if sid := strings.TrimSpace(asString(cur["secret_id"])); sid != "" && !hasNonEmptyString(pat, "secret_id") {
			pat["secret_id"] = sid
		}
		delete(pat, "secret_key")
	}
	mergeHACandidateSecrets(cur, pat)
	out, err := json.Marshal(pat)
	return string(out), err
}

func hasNonEmptyString(m map[string]any, key string) bool {
	v, ok := m[key]
	if !ok {
		return false
	}
	return strings.TrimSpace(asString(v)) != ""
}

func copySecretField(cur, pat map[string]any, key string) {
	if v, ok := cur[key]; ok && strings.TrimSpace(asString(v)) != "" {
		pat[key] = v
	}
}

func mergeHACandidateSecrets(cur, pat map[string]any) {
	curCands, _ := cur["ha_candidates"].([]any)
	patCands, ok := pat["ha_candidates"].([]any)
	if !ok || len(patCands) == 0 {
		return
	}
	curByName := map[string]map[string]any{}
	for _, item := range curCands {
		cm, ok := item.(map[string]any)
		if !ok {
			continue
		}
		name := strings.TrimSpace(asString(cm["name"]))
		if name != "" {
			curByName[name] = cm
		}
	}
	for i, item := range patCands {
		cm, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if hasNonEmptyString(cm, "api_key") {
			continue
		}
		var src map[string]any
		name := strings.TrimSpace(asString(cm["name"]))
		if name != "" {
			src = curByName[name]
		}
		if src == nil && i < len(curCands) {
			src, _ = curCands[i].(map[string]any)
		}
		if src != nil {
			copySecretField(src, cm, "api_key_enc")
			delete(cm, "api_key")
		}
		patCands[i] = cm
	}
	pat["ha_candidates"] = patCands
}

func (c *CredentialCrypto) RevealCredentialsFromConfig(ctx context.Context, cfg string) (ProviderCredentialsReveal, error) {
	decrypted, err := c.DecryptConfigJSONForRuntime(ctx, cfg)
	if err != nil {
		return ProviderCredentialsReveal{}, err
	}
	var cr struct {
		APIKey       string `json:"api_key"`
		APIKeySet    bool   `json:"api_key_set"`
		SecretKey    string `json:"secret_key"`
		HACandidates []struct {
			Name   string `json:"name"`
			APIKey string `json:"api_key"`
		} `json:"ha_candidates"`
	}
	if strings.TrimSpace(decrypted) == "" {
		return ProviderCredentialsReveal{}, nil
	}
	if err := json.Unmarshal([]byte(decrypted), &cr); err != nil {
		return ProviderCredentialsReveal{}, apierror.BadRequest("LLM_PROVIDER_MODEL", invalidProviderConfigJSONMsg)
	}
	out := ProviderCredentialsReveal{
		APIKey:       strings.TrimSpace(cr.APIKey),
		SecretKey:    strings.TrimSpace(cr.SecretKey),
		HasAPIKey:    cr.APIKeySet || cr.APIKey != "",
		HasSecretKey: cr.SecretKey != "",
	}
	for _, ha := range cr.HACandidates {
		ak := strings.TrimSpace(ha.APIKey)
		if ak == "" {
			continue
		}
		out.HACandidates = append(out.HACandidates, HACandidateCredentialReveal{
			Name:   strings.TrimSpace(ha.Name),
			APIKey: ak,
		})
	}
	return out, nil
}

func configJSONHasPlaintextSecrets(m map[string]any) bool {
	if hasNonEmptyString(m, "api_key") || hasNonEmptyString(m, "secret_key") {
		return true
	}
	cands, ok := m["ha_candidates"].([]any)
	if !ok {
		return false
	}
	for _, item := range cands {
		cm, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if hasNonEmptyString(cm, "api_key") {
			return true
		}
	}
	return false
}

func (c *CredentialCrypto) RequireKeyForPlaintext(ctx context.Context, cfg string) error {
	key, err := c.aesKey(ctx)
	if err != nil {
		return err
	}
	if len(key) == 32 {
		return nil
	}
	m, err := parseProviderConfigJSON(cfg)
	if err != nil {
		return err
	}
	if m == nil {
		return nil
	}
	if configJSONHasPlaintextSecrets(m) {
		return apierror.BadRequest("LLM_PROVIDER_MODEL", credentialKeyRequiredMsg)
	}
	return nil
}

func (c *CredentialCrypto) ProcessConfigJSONForStorage(ctx context.Context, cfg string) (string, error) {
	m, err := parseProviderConfigJSON(cfg)
	if err != nil {
		return "", err
	}
	if m == nil {
		return "", nil
	}
	key, err := c.aesKey(ctx)
	if err != nil {
		return "", err
	}
	if len(key) != 32 {
		if configJSONHasPlaintextSecrets(m) {
			return "", apierror.BadRequest("LLM_PROVIDER_MODEL", credentialKeyRequiredMsg)
		}
		out, err := json.Marshal(m)
		return string(out), err
	}
	if v, ok := m["api_key"].(string); ok && strings.TrimSpace(v) != "" {
		enc, err := c.encryptCredential(ctx, v)
		if err != nil {
			return "", err
		}
		m["api_key_enc"] = enc
		delete(m, "api_key")
		m["api_key_set"] = true
	}
	if v, ok := m["secret_key"].(string); ok && strings.TrimSpace(v) != "" {
		enc, err := c.encryptCredential(ctx, v)
		if err != nil {
			return "", err
		}
		m["secret_key_enc"] = enc
		delete(m, "secret_key")
	}
	if cands, ok := m["ha_candidates"].([]any); ok {
		for i, item := range cands {
			cm, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if ak, ok := cm["api_key"].(string); ok && strings.TrimSpace(ak) != "" {
				enc, err := c.encryptCredential(ctx, ak)
				if err != nil {
					return "", err
				}
				cm["api_key_enc"] = enc
				delete(cm, "api_key")
				cands[i] = cm
			}
		}
		m["ha_candidates"] = cands
	}
	out, err := json.Marshal(m)
	return string(out), err
}

func sanitizeConfigJSONForAPI(cfg string) string {
	cfg = strings.TrimSpace(cfg)
	if cfg == "" {
		return cfg
	}
	var m map[string]any
	if json.Unmarshal([]byte(cfg), &m) != nil {
		return cfg
	}
	if _, ok := m["api_key_enc"]; ok {
		m["api_key_set"] = true
	} else if _, ok := m["api_key_set"]; !ok {
		if ak, _ := m["api_key"].(string); strings.TrimSpace(ak) != "" {
			m["api_key_set"] = true
		}
	}
	delete(m, "api_key")
	delete(m, "api_key_enc")
	delete(m, "secret_key")
	delete(m, "secret_key_enc")
	if cands, ok := m["ha_candidates"].([]any); ok {
		for i, item := range cands {
			cm, ok := item.(map[string]any)
			if !ok {
				continue
			}
			delete(cm, "api_key")
			delete(cm, "api_key_enc")
			cands[i] = cm
		}
		m["ha_candidates"] = cands
	}
	out, _ := json.Marshal(m)
	return string(out)
}

func (c *CredentialCrypto) DecryptConfigJSONForRuntime(ctx context.Context, cfg string) (string, error) {
	cfg = strings.TrimSpace(cfg)
	if cfg == "" {
		return cfg, nil
	}
	var m map[string]any
	if json.Unmarshal([]byte(cfg), &m) != nil {
		return cfg, nil
	}
	if enc, ok := m["api_key_enc"].(string); ok && strings.TrimSpace(enc) != "" {
		if plain, err := c.decryptCredential(ctx, enc); err != nil {
			c.lg.Warn("api_key 解密失败", loggateway.StepID("credential.decrypt"), loggateway.Err(err))
		} else if plain != "" {
			m["api_key"] = plain
		}
	}
	if enc, ok := m["secret_key_enc"].(string); ok && strings.TrimSpace(enc) != "" {
		if plain, err := c.decryptCredential(ctx, enc); err != nil {
			c.lg.Warn("secret_key 解密失败", loggateway.StepID("credential.decrypt"), loggateway.Err(err))
		} else if plain != "" {
			m["secret_key"] = plain
		}
	}
	if cands, ok := m["ha_candidates"].([]any); ok {
		for i, item := range cands {
			cm, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if enc, ok := cm["api_key_enc"].(string); ok && strings.TrimSpace(enc) != "" {
				if plain, err := c.decryptCredential(ctx, enc); err != nil {
					c.lg.Warn("ha_candidate api_key 解密失败", loggateway.StepID("credential.decrypt"), loggateway.Err(err))
				} else if plain != "" {
					cm["api_key"] = plain
				}
			}
			cands[i] = cm
		}
		m["ha_candidates"] = cands
	}
	out, _ := json.Marshal(m)
	return string(out), nil
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}

func sanitizeProviderModelForAPI(m ProviderModel) ProviderModel {
	m.ConfigJSON = sanitizeConfigJSONForAPI(m.ConfigJSON)
	return m
}

func (c *CredentialCrypto) PrepareProviderModelForRuntime(ctx context.Context, m ProviderModel) (ProviderModel, error) {
	decrypted, err := c.DecryptConfigJSONForRuntime(ctx, m.ConfigJSON)
	if err != nil {
		return ProviderModel{}, err
	}
	m.ConfigJSON = decrypted
	return m, nil
}
