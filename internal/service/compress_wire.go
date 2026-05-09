package service

import (
	"net/http"
	"time"
)

// NewCompressHTTPClient is a dedicated transport for session summarization calls (shorter timeout than chat streaming).
func NewCompressHTTPClient() *http.Client {
	return &http.Client{Timeout: 120 * time.Second}
}
