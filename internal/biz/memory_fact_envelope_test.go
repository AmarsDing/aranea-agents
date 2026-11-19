package biz

import (
	"context"
	"strings"
	"testing"

	"aranea-agents/pkg/loggateway"
)

func envelopeTestCipher(t *testing.T, key byte) *CredentialFactCipher {
	t.Helper()
	k := make([]byte, 32)
	for i := range k {
		k[i] = key
	}
	crypto := NewCredentialCrypto(func(context.Context) ([]byte, error) { return k, nil }, loggateway.NewNoop())
	return NewCredentialFactCipher(crypto)
}

func TestFactEnvelope_Roundtrip(t *testing.T) {
	c := envelopeTestCipher(t, 0x2a)
	ctx := context.Background()
	enc, ok := FactEnvelopeEncrypt(ctx, c, "张三的项目代号是 Azure", "细节：跨部门协作", "fp-1")
	if !ok || enc == "" {
		t.Fatal("encrypt failed")
	}
	if strings.Contains(enc, "Azure") {
		t.Fatal("ciphertext leaks plaintext")
	}
	s, d, ok := FactEnvelopeDecrypt(ctx, c, enc, "fp-1")
	if !ok {
		t.Fatal("decrypt failed")
	}
	if s != "张三的项目代号是 Azure" || d != "细节：跨部门协作" {
		t.Fatalf("roundtrip mismatch: %q / %q", s, d)
	}
}

func TestFactEnvelope_AADBindsFingerprint(t *testing.T) {
	c := envelopeTestCipher(t, 0x2a)
	ctx := context.Background()
	enc, ok := FactEnvelopeEncrypt(ctx, c, "statement", "details", "fp-correct")
	if !ok {
		t.Fatal("encrypt failed")
	}
	if _, _, ok := FactEnvelopeDecrypt(ctx, c, enc, "fp-wrong"); ok {
		t.Fatal("decrypt with wrong AAD must fail")
	}
	// 跨 scope 挪移密文 = AAD 不符，必须失败。
	if _, _, ok := FactEnvelopeDecrypt(ctx, c, enc, ""); ok {
		t.Fatal("decrypt with empty AAD must fail")
	}
}

func TestFactEnvelope_TamperedCiphertextFails(t *testing.T) {
	c := envelopeTestCipher(t, 0x2a)
	ctx := context.Background()
	enc, ok := FactEnvelopeEncrypt(ctx, c, "statement", "details", "fp")
	if !ok {
		t.Fatal("encrypt failed")
	}
	tampered := enc[:len(enc)-2] + "AA"
	if tampered == enc {
		tampered = enc[:len(enc)-2] + "BB"
	}
	if _, _, ok := FactEnvelopeDecrypt(ctx, c, tampered, "fp"); ok {
		t.Fatal("decrypt of tampered ciphertext must fail")
	}
}

func TestFactEnvelope_WrongKeyFails(t *testing.T) {
	c1 := envelopeTestCipher(t, 0x2a)
	c2 := envelopeTestCipher(t, 0x99)
	ctx := context.Background()
	enc, ok := FactEnvelopeEncrypt(ctx, c1, "statement", "details", "fp")
	if !ok {
		t.Fatal("encrypt failed")
	}
	if _, _, ok := FactEnvelopeDecrypt(ctx, c2, enc, "fp"); ok {
		t.Fatal("decrypt with wrong key must fail")
	}
}

func TestFactEnvelope_NilDegrades(t *testing.T) {
	ctx := context.Background()
	if enc, ok := FactEnvelopeEncrypt(ctx, nil, "s", "d", "fp"); ok || enc != "" {
		t.Fatalf("nil cipher encrypt must return ok=false, got %q ok=%v", enc, ok)
	}
	// 空信封 = 无信封行，直走明文载体（ok=true 空明文）。
	if s, d, ok := FactEnvelopeDecrypt(ctx, nil, "", "fp"); !ok || s != "" || d != "" {
		t.Fatalf("empty envelope must be ok with empty plaintext, got %q/%q ok=%v", s, d, ok)
	}
	if _, _, ok := FactEnvelopeDecrypt(ctx, nil, "c2VhbGVk", "fp"); ok {
		t.Fatal("non-empty envelope with nil cipher must fail (caller falls back)")
	}
	// 空 statement 不写信封。
	c := envelopeTestCipher(t, 0x2a)
	if enc, ok := FactEnvelopeEncrypt(ctx, c, "  ", "d", "fp"); ok || enc != "" {
		t.Fatalf("blank statement must not produce envelope, got %q ok=%v", enc, ok)
	}
}

func TestMapFactKindToken_Note(t *testing.T) {
	if got := mapFactKindToken("note"); got != "note" {
		t.Fatalf("note kind must map to itself, got %q", got)
	}
	if got := mapFactKindToken("NOTE"); got != "note" {
		t.Fatalf("NOTE must canonicalize to note, got %q", got)
	}
	if vagueFactKind("note") {
		t.Fatal("note must not be a vague kind (heuristics must not reroute it)")
	}
}
