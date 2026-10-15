package data

import (
	"aranea-agents/pkg/loggateway"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"
)

func memBoolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func decodeJSONStringSlice(raw string, lg loggateway.Logger) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		lg.Warn("session memory json unmarshal failed", loggateway.StepID("data.sessionmemory"), loggateway.Err(err))
		return nil
	}
	return out
}

func decodeJSONObject(raw string, lg loggateway.Logger) map[string]any {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		lg.Warn("session memory json unmarshal failed", loggateway.StepID("data.sessionmemory"), loggateway.Err(err))
		return nil
	}
	return out
}

func decodeJSONFloatMap(raw string, lg loggateway.Logger) map[string]float64 {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" || raw == "{}" {
		return map[string]float64{}
	}
	var out map[string]float64
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		lg.Warn("session memory json unmarshal failed", loggateway.StepID("data.sessionmemory"), loggateway.Err(err))
		return map[string]float64{}
	}
	return out
}

func nameReplacePattern(oldName string) (*regexp.Regexp, error) {
	return regexp.Compile(`(?i)\b` + regexp.QuoteMeta(oldName) + `\b`)
}

func replaceNameWordBoundary(text string, re *regexp.Regexp, newName string) string {
	if text == "" || re == nil {
		return text
	}
	return re.ReplaceAllString(text, newName)
}

func composeTurnRef(sessionID, messageID string) string {
	sessionID = strings.TrimSpace(sessionID)
	messageID = strings.TrimSpace(messageID)
	switch {
	case sessionID != "" && messageID != "":
		return sessionID + ":" + messageID
	case messageID != "":
		return messageID
	default:
		return sessionID
	}
}

func jsonStrMap(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func mergeCascadeFactMeta(base, oldName, newName string, lg loggateway.Logger) string {
	var m map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(base)), &m); err != nil {
		lg.Warn("session memory json unmarshal failed", loggateway.StepID("data.sessionmemory"), loggateway.Err(err))
		m = map[string]any{}
	} else if m == nil {
		m = map[string]any{}
	}
	m["cascade_renamed_from"] = oldName
	m["cascade_renamed_to"] = newName
	m["source"] = "cascade_approve"
	b, mErr := json.Marshal(m)
	if mErr != nil {
		lg.Warn("session memory json marshal failed", loggateway.StepID("data.sessionmemory"), loggateway.Err(mErr))
		return base
	}
	return string(b)
}

func mergeCascadeReviewNote(metaJSON, status, note string, lg loggateway.Logger) string {
	var m map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(metaJSON)), &m); err != nil {
		lg.Warn("session memory json unmarshal failed", loggateway.StepID("data.sessionmemory"), loggateway.Err(err))
		m = map[string]any{}
	} else if m == nil {
		m = map[string]any{}
	}
	m["review_status"] = status
	m["review_note"] = note
	b, mErr := json.Marshal(m)
	if mErr != nil {
		lg.Warn("session memory json marshal failed", loggateway.StepID("data.sessionmemory"), loggateway.Err(mErr))
		return metaJSON
	}
	return string(b)
}

func anyStr(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return strings.TrimSpace(fmt.Sprint(v))
}

func factIDFromRow(raw []byte, fallback string) string {
	if id := strings.TrimSpace(fallback); id != "" {
		return id
	}
	var row map[string]any
	if err := json.Unmarshal(raw, &row); err == nil {
		if id, _ := row["id"].(string); strings.TrimSpace(id) != "" {
			return id
		}
	}
	return fallback
}

func encodeFloat32Blob(v []float32) []byte {
	b := make([]byte, len(v)*4)
	for i, f := range v {
		binary.LittleEndian.PutUint32(b[i*4:], math.Float32bits(f))
	}
	return b
}

func decodeFloat32Blob(b []byte) []float32 {
	if len(b) < 4 {
		return nil
	}
	n := len(b) / 4
	out := make([]float32, n)
	for i := 0; i < n; i++ {
		out[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return out
}

func vectorL2Norm(v []float32) float64 {
	var sum float64
	for _, f := range v {
		x := float64(f)
		sum += x * x
	}
	return math.Sqrt(sum)
}

func cosineSimilarity(a, b []float32) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		af := float64(a[i])
		bf := float64(b[i])
		dot += af * bf
		na += af * af
		nb += bf * bf
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

func relationValidAt(validFrom, validTo, createdAt, queryTime string) bool {
	q, ok := parseMemoryTime(queryTime)
	if !ok {
		return true
	}
	fromText := strings.TrimSpace(validFrom)
	if fromText == "" {
		fromText = strings.TrimSpace(createdAt)
	}
	if fromText != "" {
		if from, ok := parseMemoryTime(fromText); ok && q.Before(from) {
			return false
		}
	}
	toText := strings.TrimSpace(validTo)
	if toText != "" {
		if to, ok := parseMemoryTime(toText); ok && q.After(to) {
			return false
		}
	}
	return true
}

func parseMemoryTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, true
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, true
	}
	return time.Time{}, false
}

func defaultQueryTimeRFC3339(queryAt string) string {
	if strings.TrimSpace(queryAt) != "" {
		return strings.TrimSpace(queryAt)
	}
	return time.Now().UTC().Format(time.RFC3339Nano)
}
