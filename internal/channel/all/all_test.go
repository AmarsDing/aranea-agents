package all_test

import (
	"testing"

	_ "aranea-agents/internal/channel/all"
)

func TestImportAllChannels(t *testing.T) {
}

func TestAllPackageCompiles(t *testing.T) {
	var nilVal *int
	if nilVal != nil {
		t.Fatal("nil check failed")
	}
}
