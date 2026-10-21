package data

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func truncateRunes(s string, n int) string {
	if n <= 0 || s == "" {
		return s
	}
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n])
}

// normalizeEvalTimestamp accepts RFC3339(Nano) strings or epoch seconds/millis
// and returns RFC3339Nano; unparseable input yields "" (repo defaults to now).
func normalizeEvalTimestamp(ts string) string {
	ts = strings.TrimSpace(ts)
	if ts == "" {
		return ""
	}
	if t, err := time.Parse(time.RFC3339Nano, ts); err == nil {
		return t.UTC().Format(time.RFC3339Nano)
	}
	if n, err := strconv.ParseInt(ts, 10, 64); err == nil {
		if n > 1e12 {
			return time.UnixMilli(n).UTC().Format(time.RFC3339Nano)
		}
		return time.Unix(n, 0).UTC().Format(time.RFC3339Nano)
	}
	return ""
}

// evalPrevValue records one superseded statement on the new fact's metadata
// (AMC C3-A Q7 governance audit): {"statement":旧句,"valid_until":now}. It is
// written by both the same-slot and the vector-correction supersede paths.
type evalPrevValue struct {
	Statement  string `json:"statement"`
	ValidUntil string `json:"valid_until"`
}

// evalMetadataJSON builds the metadata payload for one stored atom (AMC C3-A
// Q4/Q5): message_index/atom_index locate the atom inside the request,
// event_dates carries the resolved absolute dates of the first atom's
// relative-day expressions (statement text and valid_from stay untouched),
// previous_values is the supersede audit trail.
func evalMetadataJSON(sessionID string, msgIdx, atomIdx int, eventDates []string, prev []evalPrevValue) string {
	m := map[string]any{
		"eval_session":  sessionID,
		"eval_seq":      msgIdx,
		"message_index": msgIdx,
		"atom_index":    atomIdx,
	}
	if len(eventDates) > 0 {
		m["event_dates"] = eventDates
	}
	if len(prev) > 0 {
		m["previous_values"] = prev
	}
	b, err := json.Marshal(m)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// stripEvalRolePrefix removes a leading "role: " prefix (<=16 chars) so
// relative-date resolution runs on the bare statement (AMC C3-A Q5).
func stripEvalRolePrefix(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, ": "); i >= 0 && i <= 16 {
		return strings.TrimSpace(s[i+2:])
	}
	return s
}

// evalDayPart returns the YYYY-MM-DD (UTC) date part of an RFC3339(Nano)
// timestamp; an already date-shaped value is returned as-is.
func evalDayPart(ts string) string {
	ts = strings.TrimSpace(ts)
	if ts == "" {
		return ""
	}
	if len(ts) == 10 {
		if _, err := time.Parse("2006-01-02", ts); err == nil {
			return ts
		}
	}
	if t, err := time.Parse(time.RFC3339Nano, ts); err == nil {
		return t.UTC().Format("2006-01-02")
	}
	return ""
}

// EvalEvidenceDate picks the evidence day for prefix/sort/range purposes:
// metadata.event_dates[0] first, then valid_from, then created_at (AMC C3-A
// Q6/Q9).
func EvalEvidenceDate(metadataJSON, validFrom, createdAt string) string {
	if day := evalEventDatesFirst(metadataJSON); day != "" {
		return day
	}
	if day := evalDayPart(validFrom); day != "" {
		return day
	}
	return evalDayPart(createdAt)
}

// evalEventDatesFirst reads metadata.event_dates[0] from the row metadata.
func evalEventDatesFirst(metadataJSON string) string {
	metadataJSON = strings.TrimSpace(metadataJSON)
	if metadataJSON == "" || metadataJSON == "{}" {
		return ""
	}
	var m struct {
		EventDates []string `json:"event_dates"`
	}
	if json.Unmarshal([]byte(metadataJSON), &m) != nil || len(m.EventDates) == 0 {
		return ""
	}
	return evalDayPart(m.EventDates[0])
}

// evalItemPrefix renders the "[YYYY-MM-DD · sess-<id>] " evidence prefix
// (AMC C3-A Q9). The session infix is omitted when the source session is
// unknown, and the whole prefix is omitted without a usable date.
func evalItemPrefix(day, sessionID string) string {
	day = strings.TrimSpace(day)
	sessionID = strings.TrimSpace(sessionID)
	if day == "" && sessionID == "" {
		return ""
	}
	if day == "" {
		return "[sess-" + sessionID + "] "
	}
	if sessionID == "" {
		return "[" + day + "] "
	}
	return "[" + day + " · sess-" + sessionID + "] "
}

var (
	// evalPIIIDCardRe matches 18-char resident ID numbers (17 digits + digit/X).
	evalPIIIDCardRe = regexp.MustCompile(`\b\d{17}[\dXx]\b`)
	// evalPIIBankCardRe matches 13–19 consecutive-digit card numbers.
	evalPIIBankCardRe = regexp.MustCompile(`\b\d{13,19}\b`)
	// evalPIIPhoneRe matches mainland mobile numbers (1[3-9]xxxxxxxxx).
	evalPIIPhoneRe = regexp.MustCompile(`\b1[3-9]\d{9}\b`)
)

func evalQueryAsksAbout(query string, words ...string) bool {
	q := strings.ToLower(query)
	for _, w := range words {
		if strings.Contains(q, w) {
			return true
		}
	}
	return false
}

// MaskEvalEvidence deterministically masks high-entropy identifiers in content
// when the query does not explicitly ask about that category (AMC C3-A Q9):
// 18-char ID numbers (middle 8 masked), 1[3-9]xxxxxxxxx phone numbers (middle
// 4), and 13–19 digit bank card numbers (middle 6+). Emails, addresses and
// preferences are never masked.
func MaskEvalEvidence(content, query string) string {
	out := content
	if !evalQueryAsksAbout(query, "身份证", "id number") {
		out = evalPIIIDCardRe.ReplaceAllStringFunc(out, func(m string) string {
			return m[:6] + "********" + m[len(m)-4:]
		})
	}
	if !evalQueryAsksAbout(query, "卡号", "card") {
		askID := evalQueryAsksAbout(query, "身份证", "id number")
		out = evalPIIBankCardRe.ReplaceAllStringFunc(out, func(m string) string {
			// An 18-digit ID-card-shaped number stays readable when the query
			// asks about 身份证 (otherwise the card rule would re-mask what the
			// ID rule deliberately left visible).
			if askID && evalPIIIDCardRe.MatchString(m) {
				return m
			}
			n := len(m)
			const keepHead = 4
			tail := 4
			stars := n - keepHead - tail
			if stars < 6 {
				stars = 6
				tail = n - keepHead - stars
				if tail < 0 {
					tail = 0
					stars = n - keepHead
				}
			}
			return m[:keepHead] + strings.Repeat("*", stars) + m[n-tail:]
		})
	}
	if !evalQueryAsksAbout(query, "手机号", "电话", "phone") {
		out = evalPIIPhoneRe.ReplaceAllStringFunc(out, func(m string) string {
			return m[:3] + "****" + m[len(m)-4:]
		})
	}
	return out
}
