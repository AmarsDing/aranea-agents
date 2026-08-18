package service

import (
	"errors"
	"testing"
	"time"

	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/provider"
)

func TestWrapLLMFailure_firstByteTimeout(t *testing.T) {
	err := wrapLLMFailure(chatagent.ErrFirstByteTimeout, 30*time.Second)
	if TurnErrorCodeFromErr(err) != TurnErrFirstByteTimeout {
		t.Fatalf("code = %q", TurnErrorCodeFromErr(err))
	}
}

func TestWrapLLMFailure_billing(t *testing.T) {
	err := wrapLLMFailure(errors.New("Error: Insufficient Balance"), 0)
	if TurnErrorCodeFromErr(err) != TurnErrProviderBilling {
		t.Fatalf("code = %q, want PROVIDER_BILLING", TurnErrorCodeFromErr(err))
	}
}

func TestTurnCodeFromFailure(t *testing.T) {
	if turnCodeFromFailure(provider.Failure{Kind: provider.FailureAuth}) != TurnErrProviderAuth {
		t.Fatal("auth mapping")
	}
	if turnCodeFromFailure(provider.Failure{Kind: provider.FailureStall}) != TurnErrFirstByteTimeout {
		t.Fatal("stall mapping")
	}
}
