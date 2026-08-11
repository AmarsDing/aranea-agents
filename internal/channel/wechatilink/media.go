package wechatilink

import (
	"bytes"
	"context"
	"crypto/aes"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// aesEncrypt/aesDecrypt implement AES-128-ECB with PKCS7 padding, the cipher
// used by the WeChat CDN for media payloads (key = CDNMedia.EncryptionKey).
func aesEncrypt(plaintext, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	padded := pkcs7Pad(plaintext, aes.BlockSize)
	encrypted := make([]byte, len(padded))
	for i := 0; i < len(padded); i += aes.BlockSize {
		block.Encrypt(encrypted[i:i+aes.BlockSize], padded[i:i+aes.BlockSize])
	}
	return encrypted, nil
}

func aesDecrypt(ciphertext, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	if len(ciphertext) == 0 || len(ciphertext)%aes.BlockSize != 0 {
		return nil, errors.New("wechat_ilink: ciphertext not aligned to block size")
	}
	decrypted := make([]byte, len(ciphertext))
	for i := 0; i < len(ciphertext); i += aes.BlockSize {
		block.Decrypt(decrypted[i:i+aes.BlockSize], ciphertext[i:i+aes.BlockSize])
	}
	return pkcs7Unpad(decrypted)
}

func pkcs7Pad(data []byte, blockSize int) []byte {
	padLen := blockSize - len(data)%blockSize
	padding := bytes.Repeat([]byte{byte(padLen)}, padLen)
	return append(data, padding...)
}

func pkcs7Unpad(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, errors.New("wechat_ilink: empty data")
	}
	padLen := int(data[len(data)-1])
	if padLen == 0 || padLen > aes.BlockSize || padLen > len(data) {
		return nil, errors.New("wechat_ilink: invalid pkcs7 padding")
	}
	for _, b := range data[len(data)-padLen:] {
		if int(b) != padLen {
			return nil, errors.New("wechat_ilink: inconsistent pkcs7 padding bytes")
		}
	}
	return data[:len(data)-padLen], nil
}

type uploadURLResp struct {
	Ret     int    `json:"ret"`
	CDNURL  string `json:"cdn_url"`
	ErrCode int    `json:"errcode"`
	ErrMsg  string `json:"errmsg"`
}

// GetUploadURL asks iLink for a fresh CDN upload endpoint.
func (c *client) GetUploadURL(ctx context.Context) (string, error) {
	req := struct {
		BaseInfo baseInfo `json:"base_info"`
	}{BaseInfo: baseInfo{ChannelVersion: channelVersion}}
	resp, err := c.post(ctx, "/ilink/bot/getuploadurl", req)
	if err != nil {
		return "", fmt.Errorf("wechat_ilink: getuploadurl: %w", err)
	}
	r, err := decodeJSON[uploadURLResp](resp)
	if err != nil {
		return "", err
	}
	if isSessionExpired(r.ErrCode) {
		return "", ErrSessionExpired
	}
	if r.Ret != 0 || r.ErrCode != 0 {
		return "", fmt.Errorf("wechat_ilink: getuploadurl failed: ret=%d errcode=%d msg=%s", r.Ret, r.ErrCode, r.ErrMsg)
	}
	return r.CDNURL, nil
}

// UploadToCDN pushes (already encrypted) bytes to the CDN endpoint.
func (c *client) UploadToCDN(ctx context.Context, cdnURL string, data []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cdnURL, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("wechat_ilink: cdn upload: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode >= 400 {
		return fmt.Errorf("wechat_ilink: cdn upload http %d", resp.StatusCode)
	}
	return nil
}

// DownloadFromCDN fetches raw (encrypted) bytes of a media payload.
func (c *client) DownloadFromCDN(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("wechat_ilink: cdn download: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil, fmt.Errorf("wechat_ilink: cdn download http %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}
