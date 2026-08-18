package provider

import (
	"errors"
	"net/http"
	"testing"
)

func TestClassifyFailure_billingMarkers(t *testing.T) {
	cases := []string{
		"Insufficient Balance",
		"Error: insufficient_quota",
		"账户余额不足，请充值",
		"Payment Required: 欠费",
	}
	for _, msg := range cases {
		got := ClassifyFailure(msg, nil)
		if got.Kind != FailureBilling {
			t.Errorf("ClassifyFailure(%q).Kind = %q, want billing", msg, got.Kind)
		}
		if got.Retryable {
			t.Errorf("ClassifyFailure(%q) should not be retryable", msg)
		}
		if got.Notice != NoticeLLMBilling {
			t.Errorf("ClassifyFailure(%q).Notice = %q, want %q", msg, got.Notice, NoticeLLMBilling)
		}
	}
}

func TestClassifyFailure_stall(t *testing.T) {
	got := ClassifyFailure("", errors.New("first byte timeout after 30s"))
	if got.Kind != FailureStall {
		t.Fatalf("Kind = %q, want stall", got.Kind)
	}
	if got.Notice != NoticeLLMStall {
		t.Fatalf("Notice = %q, want %q", got.Notice, NoticeLLMStall)
	}
}

func TestClassifyFailure_auth(t *testing.T) {
	got := ClassifyFailure("Authentication Fails, invalid api key", nil)
	if got.Kind != FailureAuth {
		t.Fatalf("Kind = %q, want auth", got.Kind)
	}
}

func TestClassifyHTTPFailure_402(t *testing.T) {
	got := ClassifyHTTPFailure(http.StatusPaymentRequired, "")
	if got.Kind != FailureBilling {
		t.Fatalf("Kind = %q, want billing", got.Kind)
	}
}

func TestClassifyHTTPFailure_401BillingBody(t *testing.T) {
	got := ClassifyHTTPFailure(http.StatusUnauthorized, `{"error":{"message":"Insufficient Balance"}}`)
	if got.Kind != FailureBilling {
		t.Fatalf("401 + billing body Kind = %q, want billing", got.Kind)
	}
}

func TestClassifyHTTPFailure_401Auth(t *testing.T) {
	got := ClassifyHTTPFailure(http.StatusUnauthorized, "invalid api key")
	if got.Kind != FailureAuth {
		t.Fatalf("Kind = %q, want auth", got.Kind)
	}
}
