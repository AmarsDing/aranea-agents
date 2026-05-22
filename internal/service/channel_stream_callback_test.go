package service

import (
	"context"
	"errors"
	"testing"

	chatagent "aranea-agents/internal/agent"
)

func TestStreamPreviewTurnError(t *testing.T) {
	ctx := WithChannelStreamCallback(context.Background(), func(string) error { return nil })
	if err := streamPreviewTurnError(ctx, chatagent.EventStreamResult{HasError: true, LastError: "boom"}); err == nil {
		t.Fatal("expected error when stream callback active")
	}
	if err := streamPreviewTurnError(context.Background(), chatagent.EventStreamResult{HasError: true}); err != nil {
		t.Fatal("expected nil without stream callback")
	}
	if err := streamPreviewTurnError(ctx, chatagent.EventStreamResult{}); err != nil {
		t.Fatal("expected nil without HasError")
	}
	err := streamPreviewTurnError(ctx, chatagent.EventStreamResult{HasError: true, LastError: "telegram edit failed"})
	if err == nil || err.Error() != "telegram edit failed" {
		t.Fatalf("got %v", err)
	}
	err = streamPreviewTurnError(ctx, chatagent.EventStreamResult{HasError: true})
	if err == nil || err.Error() != "stream preview update failed" {
		t.Fatalf("default detail: %v", err)
	}
}

func TestStreamPreviewTurnErrorNilWhenNoCallback(t *testing.T) {
	err := streamPreviewTurnError(context.Background(), chatagent.EventStreamResult{HasError: true, LastError: "x"})
	if err != nil {
		t.Fatal(errors.New("expected nil"))
	}
}
