package mvp

import (
	"os"
	"path/filepath"
	"sync"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/commerce"
	"aranea-agents/internal/biz/content"
	"aranea-agents/internal/biz/publish"
	"aranea-agents/internal/biz/review"
)

// Runtime holds MVP media/commerce usecases.
type Runtime struct {
	Content     *content.Usecase
	Review      *review.Usecase
	Publish     *publish.Usecase
	Opportunity *commerce.OpportunityUsecase
	Sourcing    *commerce.SourcingUsecase
	WebhookLog  []map[string]any
	// GraphResume is optional: resume Graph execution after review decision.
	GraphResume GraphResumeFn
	// GraphTaskComplete is optional: complete GraphTask so OnTaskCompleted resumes.
	GraphTaskComplete GraphTaskCompleteFn
}

// GraphResumeFn resumes a waiting_human Graph execution after review decision.
type GraphResumeFn func(executionID string, resumeValue map[string]any) error

// GraphTaskCompleteFn submits a GraphTask result (approve/reject) so existing
// GraphTaskRuntime.OnTaskCompleted can ResumeExecution.
type GraphTaskCompleteFn func(taskID, submitter, output, summary string) error

// Deps configures Ent (or other) repos for production admin wiring.
type Deps struct {
	Content           content.Repo
	Review            review.Repo
	Publish           publish.Repo
	Opportunity       commerce.OpportunityRepo
	Supplier          commerce.SupplierRepo
	Artifacts         biz.ArtifactSaver
	PackageRoot       string
	GraphResume       GraphResumeFn
	GraphTaskComplete GraphTaskCompleteFn
}

var (
	mu   sync.Mutex
	inst *Runtime
)

// Ensure returns the process singleton (lazy memory stack until Configure).
func Ensure() *Runtime {
	mu.Lock()
	defer mu.Unlock()
	if inst != nil {
		return inst
	}
	inst = newMemoryRuntime()
	return inst
}

func newMemoryRuntime() *Runtime {
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
	return rt
}

// Configure replaces the singleton with Ent-backed (or injected) repos.
// Safe to call once at admin boot; subsequent calls replace the stack.
func Configure(deps Deps) *Runtime {
	mu.Lock()
	defer mu.Unlock()
	root := deps.PackageRoot
	if root == "" {
		root = os.Getenv("ARANEA_PUBLISH_PACKAGE_ROOT")
	}
	if root == "" {
		root = filepath.Join(os.TempDir(), "aranea-publish-packages")
	}
	contentRepo := deps.Content
	if contentRepo == nil {
		contentRepo = content.NewMemoryRepo()
	}
	reviewRepo := deps.Review
	if reviewRepo == nil {
		reviewRepo = review.NewMemoryRepo()
	}
	publishRepo := deps.Publish
	if publishRepo == nil {
		publishRepo = publish.NewMemoryRepo()
	}
	oppRepo := deps.Opportunity
	if oppRepo == nil {
		oppRepo = commerce.NewMemoryOpportunityRepo()
	}
	supRepo := deps.Supplier
	if supRepo == nil {
		supRepo = commerce.NewMemorySupplierRepo()
	}
	pubUC := publish.NewUsecase(publishRepo, root)
	if deps.Artifacts != nil {
		pubUC.SetArtifactSaver(deps.Artifacts)
	}
	rt := &Runtime{
		Content:           content.NewUsecase(contentRepo),
		Review:            review.NewUsecase(reviewRepo),
		Publish:           pubUC,
		Opportunity:       commerce.NewOpportunityUsecase(oppRepo),
		Sourcing:          commerce.NewSourcingUsecase(supRepo),
		GraphResume:       deps.GraphResume,
		GraphTaskComplete: deps.GraphTaskComplete,
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

// ConfigureGraphHooks attaches Graph resume callbacks without replacing repos.
func ConfigureGraphHooks(resume GraphResumeFn, complete GraphTaskCompleteFn) {
	mu.Lock()
	defer mu.Unlock()
	if inst == nil {
		inst = newMemoryRuntime()
	}
	if resume != nil {
		inst.GraphResume = resume
	}
	if complete != nil {
		inst.GraphTaskComplete = complete
	}
}
