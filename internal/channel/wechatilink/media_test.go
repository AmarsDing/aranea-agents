package wechatilink

import (
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"aranea-agents/pkg/loggateway"
)

func TestAESRoundtrip(t *testing.T) {
	key := make([]byte, 16)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	plaintext := []byte("hello world, this is a test message for aes encryption")

	encrypted, err := aesEncrypt(plaintext, key)
	if err != nil {
		t.Fatal(err)
	}
	decrypted, err := aesDecrypt(encrypted, key)
	if err != nil {
		t.Fatal(err)
	}
	if string(decrypted) != string(plaintext) {
		t.Errorf("roundtrip mismatch: got %q", decrypted)
	}
}

func TestAESDecryptBadPadding(t *testing.T) {
	key := make([]byte, 16)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	if _, err := aesDecrypt([]byte("not-aligned"), key); err == nil {
		t.Error("want error for non-block-aligned ciphertext")
	}
}

func TestPKCS7UnpadStrict(t *testing.T) {
	// padLen 超过块长（16）必须报错，而非静默截断
	oversized := make([]byte, 32)
	oversized[31] = 17
	if _, err := pkcs7Unpad(oversized); err == nil {
		t.Error("want error for padLen > blockSize")
	}
	// 填充字节不一致必须报错
	inconsistent := make([]byte, 16)
	inconsistent[15] = 4 // 声称 4 字节填充，但前 3 字节为 0
	if _, err := pkcs7Unpad(inconsistent); err == nil {
		t.Error("want error for inconsistent padding bytes")
	}
	// 合法填充仍可通过
	valid := []byte("hello world!") // 12 字节
	padded := pkcs7Pad(valid, 16)
	got, err := pkcs7Unpad(padded)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(valid) {
		t.Errorf("valid padding rejected: got %q", got)
	}
}

func TestCDNUploadDownload(t *testing.T) {
	var uploaded []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ilink/bot/getuploadurl":
			_ = json.NewEncoder(w).Encode(map[string]any{"ret": 0, "cdn_url": "http://" + r.Host + "/cdn/upload"})
		case "/cdn/upload":
			buf := make([]byte, r.ContentLength)
			_, _ = r.Body.Read(buf)
			uploaded = buf
			w.WriteHeader(http.StatusOK)
		case "/cdn/file":
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write([]byte("media-bytes"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	c := newClient(server.URL, "tk", loggateway.NewNoop())

	cdnURL, err := c.GetUploadURL(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte("encrypted-media")
	if err := c.UploadToCDN(t.Context(), cdnURL, payload); err != nil {
		t.Fatal(err)
	}
	if string(uploaded) != string(payload) {
		t.Errorf("uploaded bytes mismatch: got %q", uploaded)
	}

	got, err := c.DownloadFromCDN(t.Context(), server.URL+"/cdn/file")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "media-bytes" {
		t.Errorf("downloaded bytes mismatch: got %q", got)
	}
}
