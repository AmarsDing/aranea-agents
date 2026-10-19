package data

import (
	"encoding/json"
	"strings"
	"time"
)

func mvpEncodeStrings(ss []string) string {
	if ss == nil {
		ss = []string{}
	}
	b, err := json.Marshal(ss)
	if err != nil {
		return "[]"
	}
	return string(b)
}

func mvpDecodeStrings(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" || s == "null" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil
	}
	return out
}

func mvpTimeString(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func mvpParseTime(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t
	}
	return time.Time{}
}

func mvpParseTimePtr(s string) *time.Time {
	t := mvpParseTime(s)
	if t.IsZero() {
		return nil
	}
	return &t
}
