package biz

import "testing"

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
