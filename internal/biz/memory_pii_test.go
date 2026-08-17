package biz

import (
	"strings"
	"testing"
)

func TestScanPII_email(t *testing.T) {
	res := ScanPII("Contact me at alice@example.com for details")
	if !res.PIIFlag {
		t.Fatal("expected pii flag")
	}
	if res.RedactedStatement == "" || res.RedactedStatement == "Contact me at alice@example.com for details" {
		t.Fatalf("redacted=%q", res.RedactedStatement)
	}
}

func TestScanPII_clean(t *testing.T) {
	res := ScanPII("Project deadline is next Friday")
	if res.PIIFlag {
		t.Fatal("expected no pii")
	}
}

func TestScanPII_cnMobile(t *testing.T) {
	res := ScanPII("电话 13800138000")
	if !res.PIIFlag {
		t.Fatal("expected pii flag for bare CN mobile")
	}
	if strings.Contains(res.RedactedStatement, "13800138000") {
		t.Fatalf("mobile not redacted: %q", res.RedactedStatement)
	}
}

func TestScanPII_idCardNotFlaggedAsPhone(t *testing.T) {
	// 18-digit ID card must hit the id_card detector only — the CN mobile
	// detector's \b guards must prevent a partial 11-digit match inside it.
	res := ScanPII("身份证号 110101199003077777")
	if !res.PIIFlag {
		t.Fatal("expected pii flag for id card")
	}
	for _, tp := range res.PIITypes {
		if tp == "phone" {
			t.Fatalf("id card falsely flagged as phone: %v", res.PIITypes)
		}
	}
}

// R2：裸固话（0571-8899-1234）是运维值班/组织联系信息，不是个人 PII，不得脱敏。
func TestScanPII_fixedLineNotFlagged(t *testing.T) {
	res := ScanPII("运维值班电话是0571-8899-1234，仅限工作时间拨打")
	if res.PIIFlag {
		t.Fatalf("fixed-line must not be flagged, got types=%v redacted=%q", res.PIITypes, res.RedactedStatement)
	}
}

// 带横杠/空格的中国手机号仍须脱敏（删除固话分支后由扩展的 CN mobile 正则兜底）。
func TestScanPII_dashedCNMobile(t *testing.T) {
	for _, s := range []string{"电话 138-1234-5678", "电话 138 1234 5678"} {
		res := ScanPII(s)
		if !res.PIIFlag {
			t.Fatalf("expected pii flag for %q", s)
		}
		if strings.Contains(res.RedactedStatement, "1234") {
			t.Fatalf("mobile not redacted in %q: %q", s, res.RedactedStatement)
		}
	}
}

// 显式国际冠码格式仍须脱敏。
func TestScanPII_internationalPhone(t *testing.T) {
	res := ScanPII("Call me at +1 555-123-4567")
	if !res.PIIFlag {
		t.Fatal("expected pii flag for international phone")
	}
	if strings.Contains(res.RedactedStatement, "555") {
		t.Fatalf("international phone not redacted: %q", res.RedactedStatement)
	}
}
