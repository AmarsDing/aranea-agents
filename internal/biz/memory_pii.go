package biz

import (
	"regexp"
	"strings"
)

var (
	piiEmailRe    = regexp.MustCompile(`(?i)\b[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}\b`)
	piiPhoneRe    = regexp.MustCompile(`(?:\+?\d{1,3}[\s-]?)?\(?\d{2,4}\)?[\s-]?\d{3,4}[\s-]?\d{3,4}\b`)
	piiIDCardRe   = regexp.MustCompile(`\b\d{17}[\dXx]\b`)
	piiCreditRe   = regexp.MustCompile(`\b(?:\d[ -]*?){12,18}\d\b`)
)

// PIIScanResult holds detection output for one statement.
type PIIScanResult struct {
	PIIFlag           bool
	RedactedStatement string
}

// ScanPII detects common PII patterns and returns a redacted copy when matched.
func ScanPII(statement string) PIIScanResult {
	stmt := strings.TrimSpace(statement)
	if stmt == "" {
		return PIIScanResult{}
	}
	redacted := stmt
	hit := false
	for _, pair := range []struct {
		re  *regexp.Regexp
		rep string
	}{
		{piiEmailRe, "[email]"},
		{piiPhoneRe, "[phone]"},
		{piiIDCardRe, "[id]"},
		{piiCreditRe, "[card]"},
	} {
		if pair.re.MatchString(redacted) {
			hit = true
			redacted = pair.re.ReplaceAllString(redacted, pair.rep)
		}
	}
	if !hit {
		return PIIScanResult{}
	}
	return PIIScanResult{PIIFlag: true, RedactedStatement: strings.TrimSpace(redacted)}
}
