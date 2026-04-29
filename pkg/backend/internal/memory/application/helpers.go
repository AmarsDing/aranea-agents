package application

import (
	"arenea/backend/internal/kernel/errs"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"sync/atomic"
	"time"
)

func validationError(format string, args ...any) error {
	return fmt.Errorf("%w: %s", errs.ErrValidation, fmt.Sprintf(format, args...))
}

func nowUTC() string {
	return time.Now().UTC().Format(time.RFC3339)
}

var fallbackIDCounter uint64

func newID() string {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		n := atomic.AddUint64(&fallbackIDCounter, 1)
		return hex.EncodeToString([]byte(time.Now().UTC().Format("20060102150405"))) + hex.EncodeToString([]byte{byte(n >> 8), byte(n)})
	}
	return hex.EncodeToString(buf)
}

func previewText(value string, maxRunes int) string {
	value = strings.TrimSpace(value)
	if maxRunes <= 0 {
		return value
	}
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	return string(runes[:maxRunes]) + "..."
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
