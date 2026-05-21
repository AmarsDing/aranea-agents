package biz

import (
	"testing"

	"aranea-agents/internal/data/sessionmemory"
)

func TestSessionAdminStoreImplementedByDataStore(t *testing.T) {
	var _ SessionAdminStore = (*sessionmemory.Store)(nil)
}

func TestWrapSessionAdminStoreNil(t *testing.T) {
	if WrapSessionAdminStore(nil) != nil {
		t.Fatal("expected nil admin store")
	}
}
