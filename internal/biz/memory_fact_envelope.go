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

	"aranea-agents/pkg/loggateway"
)

// factEnvelopeWire 是 content_enc 承载的 JSON 线格式（D3 N-2 全量记忆信封，
// 拍板 B）。v=1 记录格式版本（换密钥/换算法时递增并保留旧版本可读）；
// s/d 分别承载权威全文 statement 与 details_markdown。
type factEnvelopeWire struct {
	V int    `json:"v"`
	S string `json:"s"`
	D string `json:"d"`
}

// FactContentCipher 是记忆事实 content_enc 信封的加解密口（data 层经此接
// CredentialCrypto，不直接依赖具体实现）。语义：空明文返回空串（行无信封）；
// 加密失败与解密失败返回 ok=false——data 层各自降级（写路径记警告写明文载体，
// 读路径回退 statement 预览），绝不把密文当明文透出。
type FactContentCipher interface {
	EncryptFactContent(ctx context.Context, statement, details, aad string) (enc string, ok bool)
	DecryptFactContent(ctx context.Context, enc, aad string) (statement, details string, ok bool)
}

// FactEnvelopeEncrypt 打包 statement+details 为信封密文（aAD 绑定指纹，防跨行
// 密文挪移）。ok=false 表示无密钥或加密失败——调用方按降级策略处理。
func FactEnvelopeEncrypt(ctx context.Context, c FactContentCipher, statement, details, aad string) (string, bool) {
	if c == nil || strings.TrimSpace(statement) == "" {
		return "", false
	}
	return c.EncryptFactContent(ctx, statement, details, aad)
}

// FactEnvelopeDecrypt 解信封；enc 空串返回 ok=true 且空明文（无信封行直走
// statement 明文载体）。ok=false 仅表示解密失败（密文损坏/密钥缺失）。
func FactEnvelopeDecrypt(ctx context.Context, c FactContentCipher, enc, aad string) (statement, details string, ok bool) {
	if strings.TrimSpace(enc) == "" {
		return "", "", true
	}
	if c == nil {
		return "", "", false
	}
	return c.DecryptFactContent(ctx, enc, aad)
}

// CredentialFactCipher 用凭据模块的 CredentialCrypto（AES-GCM，密钥来自
// ARANEA_CREDENTIAL_KEY 或系统设置）实现 FactContentCipher。信封密文 = base64
// (nonce || AES-GCM(plaintext))，与 credentials 表字段级加密同构（同密钥体系）。
// AAD 绑定指纹：密文不可跨 scope/fingerprint 挪移复用。加解密失败降级由 data
// 层决定，这里只透出 ok。
type CredentialFactCipher struct {
	crypto *CredentialCrypto
}

// NewCredentialFactCipher 包装凭据加密器为记忆信封加密器；crypto 为 nil 时
// 返回 nil（data 层按无信封降级）。
func NewCredentialFactCipher(crypto *CredentialCrypto) *CredentialFactCipher {
	if crypto == nil {
		return nil
	}
	return &CredentialFactCipher{crypto: crypto}
}

func (c *CredentialFactCipher) EncryptFactContent(ctx context.Context, statement, details, aad string) (string, bool) {
	plain, err := marshalFactEnvelope(statement, details)
	if err != nil {
		return "", false
	}
	key, err := c.crypto.aesKey(ctx)
	if err != nil || len(key) != 32 {
		return "", false
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", false
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", false
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", false
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plain), []byte(aad))
	return base64.StdEncoding.EncodeToString(sealed), true
}

func (c *CredentialFactCipher) DecryptFactContent(ctx context.Context, enc, aad string) (string, string, bool) {
	key, err := c.crypto.aesKey(ctx)
	if err != nil || len(key) != 32 {
		return "", "", false
	}
	raw, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		c.warnDecrypt("base64", err)
		return "", "", false
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", "", false
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", "", false
	}
	n := gcm.NonceSize()
	if len(raw) < n {
		c.warnDecrypt("truncated", nil)
		return "", "", false
	}
	plain, err := gcm.Open(nil, raw[:n], raw[n:], []byte(aad))
	if err != nil {
		c.warnDecrypt("gcm_open", err)
		return "", "", false
	}
	s, d, ok := unmarshalFactEnvelope(string(plain))
	if !ok {
		c.warnDecrypt("wire", nil)
	}
	return s, d, ok
}

// warnDecrypt 记录信封解密失败（D3 N-2 降级契约：data 层回退明文载体列不
// crash，告警在这里打出——密文损坏/密钥缺失/AAD 不符都走这条）。
func (c *CredentialFactCipher) warnDecrypt(stage string, err error) {
	if c == nil || c.crypto == nil || c.crypto.lg == nil {
		return
	}
	lg := c.crypto.lg
	lg.Warn("记忆信封解密失败，回退明文载体列",
		loggateway.StepID("memory.fact.envelope"),
		loggateway.Str("stage", stage),
		loggateway.Err(err))
}

func marshalFactEnvelope(statement, details string) (string, error) {
	b, err := json.Marshal(factEnvelopeWire{V: 1, S: statement, D: details})
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func unmarshalFactEnvelope(plain string) (statement, details string, ok bool) {
	var w factEnvelopeWire
	if err := json.Unmarshal([]byte(plain), &w); err != nil || w.V != 1 {
		return "", "", false
	}
	return w.S, w.D, true
}
