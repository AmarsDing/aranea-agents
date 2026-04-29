package application

import (
	"crypto/rand"
	"encoding/hex"
	"sync/atomic"
	"time"
)

var fallbackIDCounter uint64

func newID() string {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		n := atomic.AddUint64(&fallbackIDCounter, 1)
		return hex.EncodeToString([]byte(time.Now().UTC().Format("20060102150405"))) + hex.EncodeToString([]byte{byte(n >> 8), byte(n)})
	}
	return hex.EncodeToString(buf)
}
