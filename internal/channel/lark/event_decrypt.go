package lark

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strings"
)

// DecryptEvent decrypts Feishu/Lark encrypted event payload (AES-256-CBC).
func DecryptEvent(encryptKey, cipherTextB64 string) ([]byte, error) {
	encryptKey = strings.TrimSpace(encryptKey)
	cipherTextB64 = strings.TrimSpace(cipherTextB64)
	if encryptKey == "" || cipherTextB64 == "" {
		return nil, errDecryptEmptyKeyOrCipher
	}
	cipherBytes, err := base64.StdEncoding.DecodeString(cipherTextB64)
	if err != nil {
		return nil, errDecryptBase64
	}
	key := sha256.Sum256([]byte(encryptKey))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	if len(cipherBytes) < aes.BlockSize {
		return nil, errCipherTooShort
	}
	iv := cipherBytes[:aes.BlockSize]
	data := cipherBytes[aes.BlockSize:]
	if len(data)%aes.BlockSize != 0 {
		return nil, errDecryptInvalidBlockSize
	}
	mode := cipher.NewCBCDecrypter(block, iv)
	plain := make([]byte, len(data))
	mode.CryptBlocks(plain, data)
	plain, err = pkcs7Unpad(plain, aes.BlockSize)
	if err != nil {
		return nil, err
	}
	return plain, nil
}

func pkcs7Unpad(data []byte, blockSize int) ([]byte, error) {
	if len(data) == 0 || len(data)%blockSize != 0 {
		return nil, errDecryptInvalidPadding
	}
	pad := int(data[len(data)-1])
	if pad <= 0 || pad > blockSize || pad > len(data) {
		return nil, errDecryptInvalidPadding
	}
	for i := 0; i < pad; i++ {
		if data[len(data)-1-i] != byte(pad) {
			return nil, errDecryptInvalidPadding
		}
	}
	return data[:len(data)-pad], nil
}

// UnwrapEncryptedWebhookBody returns decrypted JSON when body uses {"encrypt":"..."} envelope.
func UnwrapEncryptedWebhookBody(encryptKey string, raw []byte) ([]byte, error) {
	if len(raw) == 0 {
		return raw, nil
	}
	var wrap struct {
		Encrypt string `json:"encrypt"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil || strings.TrimSpace(wrap.Encrypt) == "" {
		return raw, nil
	}
	if strings.TrimSpace(encryptKey) == "" {
		return nil, errEncryptKeyMissing
	}
	return DecryptEvent(encryptKey, wrap.Encrypt)
}
