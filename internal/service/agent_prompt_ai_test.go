package service

import (
	"errors"
	"testing"
)

func TestMapPromptFileAIError(t *testing.T) {
	err := mapPromptFileAIError(errors.New("no matching model in catalog"))
	if err == nil || err.Error() == "" {
		t.Fatal("expected mapped error")
	}
	if got := mapPromptFileAIError(errors.New("instruction is required")); got == nil {
		t.Fatal("expected bad request mapping")
	}
}
