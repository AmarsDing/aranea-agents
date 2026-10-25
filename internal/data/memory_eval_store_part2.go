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
		if n >= 1e11 || n <= -1e11 {
			return time.UnixMilli(n).UTC().Format(time.RFC3339Nano)
		}
		return time.Unix(n, 0).UTC().Format(time.RFC3339Nano)
	}
	for _, layout := range []string{"2006-01-02", "2006-01-02 15:04:05", "2006-01-02 15:04", "2006-01-02 15:04:05.999999999Z07:00", "2006-01-02 15:04:05.999999999Z07"} {
		if t, err := time.Parse(layout, ts); err == nil {
			return t.UTC().Format(time.RFC3339Nano)
		}
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
	var spans struct {
		Spans []struct {
			Start string `json:"start"`
		} `json:"event_spans"`
	}
	if json.Unmarshal([]byte(metadataJSON), &spans) == nil && len(spans.Spans) > 0 {
		if day := evalDayPart(spans.Spans[0].Start); day != "" {
			return day
		}
	}
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
	evalPIIPhoneRe     = regexp.MustCompile(`\b1[3-9]\d{9}\b`)
	evalPIIPassportRe  = regexp.MustCompile(`\b[EGPegp]\d{8}\b`)
	evalPIICarPlateRe  = regexp.MustCompile(`[京津沪渝冀豫云辽黑湘皖鲁新苏浙赣鄂桂甘晋蒙陕吉闽贵粤青藏川宁琼使领][A-Za-z][·\-]?[A-Za-z0-9]{5}`)
	evalPIIEmailRe     = regexp.MustCompile(`(?i)[a-z0-9.!#$%&'*+/=?^_` + "`" + `{|}~-]+@[a-z0-9](?:[a-z0-9.-]*[a-z0-9])?\.[a-z]{2,}`)
	evalPIISSNRe       = regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`)
	evalPIISecretRe    = regexp.MustCompile(`(?i)(?:api[_ -]?key|access[_ -]?token|password|密码|密钥)\s*(?:[:=：]|\bis\b|是|为)\s*(?:"[^"\r\n]+"|'[^'\r\n]+'|[^\s,;，；。]+)`)
	evalPIIIntlPhoneRe = regexp.MustCompile(`(?:\+\d{1,3}[ -]?)?(?:\(\d{3}\)|\b\d{3})[ -]?\d{3}[ -]\d{4}\b`)
	evalPIIE164PhoneRe = regexp.MustCompile(`\+[1-9]\d{7,14}\b`)
	evalPIIStreetENRe  = regexp.MustCompile(`(?i)\b\d{1,6}\s+(?:[a-z][a-z.'-]*\s+){1,6}(?:street|st|road|rd|avenue|ave|lane|ln|boulevard|blvd|drive|dr|court|ct)\b`)
	evalPIIStreetCNRe  = regexp.MustCompile(`[\p{Han}]{2,}(?:路|街|巷|弄)\d{1,6}号(?:\d{1,5}室)?`)
)

func evalQueryAsksAbout(query string, words ...string) bool {
	q := strings.ToLower(query)
	for _, w := range words {
		if w[0] >= 128 && strings.Contains(q, w) || w[0] < 128 && regexp.MustCompile(`\b`+regexp.QuoteMeta(w)+`\b`).MatchString(q) {
			return true
		}
	}
	return false
}

// MaskEvalEvidence deterministically masks high-entropy identifiers in content
// when the query does not explicitly ask about that category (AMC C3-A Q9):
// 18-char ID numbers (middle 8 masked), 1[3-9]xxxxxxxxx phone numbers (middle
// 4), bank cards, email addresses and SSNs. Credentials are always redacted.
// A request for one identifier category does not disclose the other categories.
func MaskEvalEvidence(content, query string) string {
	out := content
	if !evalQueryAsksAbout(query, "address", "住址", "地址", "门牌") {
		out = evalPIIStreetENRe.ReplaceAllString(out, "[street address redacted]")
		out = evalPIIStreetCNRe.ReplaceAllString(out, "[street address redacted]")
	}
	if !evalQueryAsksAbout(query, "邮箱", "电子邮件", "email", "e-mail") {
		out = evalPIIEmailRe.ReplaceAllString(out, "[email redacted]")
	}
	if !evalQueryAsksAbout(query, "social security", "ssn", "社保号") {
		out = evalPIISSNRe.ReplaceAllString(out, "***-**-****")
	}
	// Credential values are not needed as memory evidence for any query.
	out = evalPIISecretRe.ReplaceAllStringFunc(out, func(value string) string {
		i := strings.IndexAny(value, ":=：")
		if i < 0 {
			return "[credential redacted]"
		}
		return value[:i] + ": [redacted]"
	})
	if !evalQueryAsksAbout(query, "身份证", "id number") {
		out = evalPIIIDCardRe.ReplaceAllStringFunc(out, func(m string) string {
			return m[:6] + "********" + m[len(m)-4:]
		})
	}
	if !evalQueryAsksAbout(query, "卡号", "bank card", "credit card", "debit card", "card number") {
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
		out = evalPIIE164PhoneRe.ReplaceAllString(out, "[phone redacted]")
		out = evalPIIIntlPhoneRe.ReplaceAllString(out, "[phone redacted]")
		out = evalPIIPhoneRe.ReplaceAllStringFunc(out, func(m string) string {
			return m[:3] + "****" + m[len(m)-4:]
		})
	}
	if !evalQueryAsksAbout(query, "护照", "passport") {
		out = evalPIIPassportRe.ReplaceAllStringFunc(out, func(m string) string {
			return m[:1] + "******" + m[len(m)-2:]
		})
	}
	if !evalQueryAsksAbout(query, "车牌", "plate", "license plate") {
		out = evalPIICarPlateRe.ReplaceAllStringFunc(out, func(m string) string {
			r := []rune(m)
			var b strings.Builder
			for i, c := range r {
				if i < 2 || c == '·' {
					b.WriteRune(c)
				} else {
					b.WriteRune('*')
				}
			}
			return b.String()
		})
	}
	return out
}
