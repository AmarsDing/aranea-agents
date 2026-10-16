package mvp

import (
	"os"
	"path/filepath"
	"sync"

	"aranea-agents/internal/biz/commerce"
	"aranea-agents/internal/biz/content"
	"aranea-agents/internal/biz/publish"
	"aranea-agents/internal/biz/review"
)

// Runtime holds MVP media/commerce usecases. Memory-backed until Ent repos wire in.
type Runtime struct {
	Content      *content.Usecase
	Review       *review.Usecase
	Publish      *publish.Usecase
	Opportunity  *commerce.OpportunityUsecase
	Sourcing     *commerce.SourcingUsecase
	WebhookLog   []map[string]any
}

var (
	mu   sync.Mutex
	inst *Runtime
)

// Ensure returns the process singleton (lazy memory stack).
func Ensure() *Runtime {
	mu.Lock()
	defer mu.Unlock()
	if inst != nil {
		return inst
	}
	root := os.Getenv("ARANEA_PUBLISH_PACKAGE_ROOT")
	if root == "" {
		root = filepath.Join(os.TempDir(), "aranea-publish-packages")
	}
	rt := &Runtime{
		Content:     content.NewUsecase(content.NewMemoryRepo()),
		Review:      review.NewUsecase(review.NewMemoryRepo()),
		Publish:     publish.NewUsecase(publish.NewMemoryRepo(), root),
		Opportunity: commerce.NewOpportunityUsecase(commerce.NewMemoryOpportunityRepo()),
		Sourcing:    commerce.NewSourcingUsecase(commerce.NewMemorySupplierRepo()),
	}
	rt.Opportunity.SetWebhook(func(eventType string, payload map[string]any) error {
		mu.Lock()
		defer mu.Unlock()
		if inst != nil {
			inst.WebhookLog = append(inst.WebhookLog, map[string]any{"type": eventType, "payload": payload})
		}
		return nil
	})
	inst = rt
	return inst
}

// ResetForTest clears the singleton (tests only).
func ResetForTest() {
	mu.Lock()
	defer mu.Unlock()
	inst = nil
}
