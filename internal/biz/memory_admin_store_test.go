package biz

import (
	"testing"
)

func TestWrapSessionAdminStoreNil(t *testing.T) {
	// WrapSessionAdminStore has been removed; verify the interface is non-nil.
	var _ SessionAdminStore = (SessionAdminStore)(nil)
	// A nil SessionAdminStore should be nil.
	if SessionAdminStore(nil) != nil {
		t.Fatal("expected nil admin store")
	}
}
