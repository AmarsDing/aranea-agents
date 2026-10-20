package biz

import (
	"sync/atomic"
	"testing"
)

func TestOnCatalogChange_FiresFromNotify(t *testing.T) {
	u := &LlmProviderModelUsecase{}
	var n atomic.Int32
	u.OnCatalogChange(func() { n.Add(1) })
	u.notifyCatalogChange()
	u.notifyCatalogChange()
	if n.Load() != 2 {
		t.Fatalf("hooks fired %d, want 2", n.Load())
	}
}

func TestOnCatalogChange_NilSafe(t *testing.T) {
	var u *LlmProviderModelUsecase
	u.OnCatalogChange(func() {})
	u.notifyCatalogChange()
	(&LlmProviderModelUsecase{}).OnCatalogChange(nil)
}
