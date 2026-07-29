package biz

import (
	"regexp"
	"strings"

	"aranea-agents/pkg/apierror"
)

var ErrPIIBlocked = apierror.Forbidden("MEMORY", "fact blocked by PII policy")

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
	piiPhoneRe      = regexp.MustCompile(`(?:\+(?:86|1|44|81|82|91|49|33|61|7)\s?)?(?:\(\d{2,4}\)\s?\d{3,4}[\s-]?\d{3,4}|\d{2,4}[\s-]\d{3,4}[\s-]\d{3,4})\b`) // Require country code prefix or structured format
	piiCNMobileRe   = regexp.MustCompile(`\b1[3-9]\d{9}\b`)                                                                                                   // Bare Chinese mobile: 11 digits, 1[3-9] prefix; \b blocks partial matches inside longer digit runs
	piiIDCardRe     = regexp.MustCompile(`\b\d{17}[\dXx]\b`)
	piiCreditRe     = regexp.MustCompile(`\b(?:[345]\d[ -]*?(?:\d[ -]*?){11,17}\d|\d[ -]*?(?:\d[ -]*?){13,18}\d)\b`) // 14-19 digits with optional spaces/dashes; leading 3/4/5 pattern first to reduce false positives
	piiBankAcctRe   = regexp.MustCompile(`\b62\d{14,17}\b`)                                                          // UnionPay only: 62 prefix + 14-17 more digits (total 16-19)
	piiSSNLikeRe    = regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`)
	piiMedicalRe    = regexp.MustCompile(`(?i)(?:medical record|patient id|mrn)\s*[:#]?\s*\S+`)
	piiHomeAddrRe   = regexp.MustCompile(`(?i)\d+\s+[A-Z][a-zA-Z]+\s+(?:Street|St|Avenue|Ave|Boulevard|Blvd|Road|Rd|Lane|Ln|Drive|Dr|Court|Ct)`)
	piiHomeAddrCNRe = regexp.MustCompile(`(?:[\p{Han}]{2,6}(?:省|市|区|县|镇|乡))?(?:[\p{Han}]{2,8}(?:路|街|道|巷|弄|胡同))?\d+(?:号|弄)\d*(?:室|栋|楼|单元)?`) // Chinese address: 省/市/区 + 路/街/道 + 号/弄
	piiSecretKeyRe  = regexp.MustCompile(`(?i)(?:api[_-]?key|secret[_-]?key|access[_-]?token)\s*[:=]\s*["']?[A-Za-z0-9_\-]{8,}["']?`)       // Exclude "password" to avoid blocking user preferences; require 8+ char value
)

type piiDetector struct {
	re      *regexp.Regexp
	rep     string
	typeTag string
}

var piiDetectors = []piiDetector{
	{piiEmailRe, "[email]", "email"},
	{piiPhoneRe, "[phone]", "phone"},
	{piiCNMobileRe, "[phone]", "phone"},
	{piiIDCardRe, "[id]", "id_card"},
	{piiBankAcctRe, "[bank_account]", "bank_account"},
	{piiCreditRe, "[card]", "credit_card"},
	{piiSSNLikeRe, "[ssn]", "ssn"},
	{piiMedicalRe, "[medical]", "medical_record"},
	{piiHomeAddrRe, "[address]", "home_address"},
	{piiHomeAddrCNRe, "[address]", "home_address"},
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
