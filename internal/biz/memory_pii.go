package biz

import (
	"regexp"
	"strings"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

var ErrPIIBlocked = kerrors.Forbidden("MEMORY", "fact blocked by PII policy")

type PIIPolicy string

const (
	PIIPolicyRedact PIIPolicy = "redact"
	PIIPolicyBlock  PIIPolicy = "block"
	PIIPolicyReview PIIPolicy = "review"
)

func DefaultPIIPolicy() PIIPolicy {
	return PIIPolicyRedact
}

func ParsePIIPolicy(s string) PIIPolicy {
	switch strings.TrimSpace(strings.ToLower(s)) {
	case "block":
		return PIIPolicyBlock
	case "review":
		return PIIPolicyReview
	default:
		return PIIPolicyRedact
	}
}

var (
	piiEmailRe      = regexp.MustCompile(`(?i)\b[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}\b`)
	piiPhoneRe      = regexp.MustCompile(`(?:\+?\d{1,3}[\s-]?)?\(?\d{2,4}\)?[\s-]?\d{3,4}[\s-]?\d{3,4}\b`)
	piiIDCardRe     = regexp.MustCompile(`\b\d{17}[\dXx]\b`)
	piiCreditRe     = regexp.MustCompile(`\b(?:\d[ -]*?){12,18}\d\b`)
	piiBankAcctRe   = regexp.MustCompile(`\b\d{8,20}\b`)
	piiSSNLikeRe    = regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`)
	piiMedicalRe    = regexp.MustCompile(`(?i)(?:medical record|patient id|mrn)\s*[:#]?\s*\S+`)
	piiHomeAddrRe   = regexp.MustCompile(`(?i)\d+\s+[A-Z][a-zA-Z]+\s+(?:Street|St|Avenue|Ave|Boulevard|Blvd|Road|Rd|Lane|Ln|Drive|Dr|Court|Ct)`)
	piiSecretKeyRe  = regexp.MustCompile(`(?i)(?:api[_-]?key|secret[_-]?key|token|password|passwd|pwd)\s*[:=]\s*\S+`)
)

type piiDetector struct {
	re      *regexp.Regexp
	rep     string
	typeTag string
}

var piiDetectors = []piiDetector{
	{piiEmailRe, "[email]", "email"},
	{piiPhoneRe, "[phone]", "phone"},
	{piiIDCardRe, "[id]", "id_card"},
	{piiCreditRe, "[card]", "credit_card"},
	{piiBankAcctRe, "[bank_account]", "bank_account"},
	{piiSSNLikeRe, "[ssn]", "ssn"},
	{piiMedicalRe, "[medical]", "medical_record"},
	{piiHomeAddrRe, "[address]", "home_address"},
	{piiSecretKeyRe, "[secret]", "secret_key"},
}

type PIIScanResult struct {
	PIIFlag           bool
	RedactedStatement string
	PIITypes          []string
}

// ScanPII detects common PII patterns and returns a redacted copy when matched.
func ScanPII(statement string) PIIScanResult {
	stmt := strings.TrimSpace(statement)
	if stmt == "" {
		return PIIScanResult{}
	}
	redacted := stmt
	hit := false
	var types []string
	for _, d := range piiDetectors {
		if d.re.MatchString(redacted) {
			hit = true
			types = append(types, d.typeTag)
			redacted = d.re.ReplaceAllString(redacted, d.rep)
		}
	}
	if !hit {
		return PIIScanResult{}
	}
	return PIIScanResult{PIIFlag: true, RedactedStatement: strings.TrimSpace(redacted), PIITypes: types}
}
