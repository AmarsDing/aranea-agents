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
