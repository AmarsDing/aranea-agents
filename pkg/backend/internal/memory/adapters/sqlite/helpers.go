// Package sqlite 实现 Memory 边界的 SQLite 持久化。由 repository 薄委托，避免循环依赖时仅传 *sql.DB。
package sqlite

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"time"
)

type scanner interface {
	Scan(dest ...any) error
}

func nowISO() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func previewText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || len([]rune(value)) <= limit {
		return value
	}
	runes := []rune(value)
	return string(runes[:limit]) + "..."
}

func normalizeJSONList(value string) string {
	if strings.TrimSpace(value) == "" {
		return "[]"
	}
	if json.Valid([]byte(value)) {
		return value
	}
	encoded, err := json.Marshal([]string{value})
	if err != nil {
		return "[]"
	}
	return string(encoded)
}

func decodeFloat32Blob(blob []byte) ([]float32, error) {
	if len(blob)%4 != 0 {
		return nil, errors.New("invalid float32 blob length")
	}
	out := make([]float32, len(blob)/4)
	for i := range out {
		out[i] = math.Float32frombits(binary.LittleEndian.Uint32(blob[i*4:]))
	}
	return out, nil
}

func vectorNorm(vec []float32) float64 {
	if len(vec) == 0 {
		return 0
	}
	var sum float64
	for _, v := range vec {
		sum += float64(v) * float64(v)
	}
	return math.Sqrt(sum)
}

func dotProduct(a, b []float32) float64 {
	if len(a) != len(b) {
		return 0
	}
	var sum float64
	for i := range a {
		sum += float64(a[i]) * float64(b[i])
	}
	return sum
}
