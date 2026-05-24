package lark

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"testing"
)

func encryptTestPayload(key, plain string) (string, error) {
	sum := sha256.Sum256([]byte(key))
	block, err := aes.NewCipher(sum[:])
	if err != nil {
		return "", err
	}
	pad := aes.BlockSize - len(plain)%aes.BlockSize
	padded := make([]byte, len(plain)+pad)
	copy(padded, plain)
	for i := len(plain); i < len(padded); i++ {
		padded[i] = byte(pad)
	}
	iv := make([]byte, aes.BlockSize)
	mode := cipher.NewCBCEncrypter(block, iv)
	out := make([]byte, len(padded))
	mode.CryptBlocks(out, padded)
	buf := append(iv, out...)
	return base64.StdEncoding.EncodeToString(buf), nil
}

func TestUnwrapEncryptedWebhookBody(t *testing.T) {
	key := "test-encrypt-key"
	plain := `{"type":"event_callback","event":{}}`
	cipherB64, err := encryptTestPayload(key, plain)
	if err != nil {
		t.Fatal(err)
	}
	wrap, _ := json.Marshal(map[string]string{"encrypt": cipherB64})
	got, err := UnwrapEncryptedWebhookBody(key, wrap)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if string(got) != plain {
		t.Fatalf("want %q got %q", plain, string(got))
	}
	unchanged, err := UnwrapEncryptedWebhookBody(key, []byte(`{"type":"plain"}`))
	if err != nil || string(unchanged) != `{"type":"plain"}` {
		t.Fatalf("plain passthrough: err=%v body=%q", err, string(unchanged))
	}
}
