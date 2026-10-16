package mvp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	// OutboundWebhook delivers opportunity.created / theme.digest to subscribed
	// webhook configs (NovaTrader intel only). Optional; memory log always kept.
	OutboundWebhook *biz.WebhookDispatcher
	// ReviewNotify is optional IM/HTTP pending-review notify (noop if nil).
	ReviewNotify review.NotifyFn
}

var (
	mu         sync.Mutex
	inst       *Runtime
	outbound   *biz.WebhookDispatcher
	notifyFn   review.NotifyFn
	resumeFn   GraphResumeFn
	completeFn GraphTaskCompleteFn
)

// Ensure returns the process singleton (lazy memory stack until Configure).
func Ensure() *Runtime {
	mu.Lock()
	defer mu.Unlock()
	if inst != nil {
		return inst
	}
	inst = newMemoryRuntime()
	applySideHooksLocked(inst)
	return inst
}

func newMemoryRuntime() *Runtime {
	root := os.Getenv("ARANEA_PUBLISH_PACKAGE_ROOT")
	if root == "" {
		root = filepath.Join(os.TempDir(), "aranea-publish-packages")
	}
	return &Runtime{
		Content:     content.NewUsecase(content.NewMemoryRepo()),
		Review:      review.NewUsecase(review.NewMemoryRepo()),
		Publish:     publish.NewUsecase(publish.NewMemoryRepo(), root),
		Opportunity: commerce.NewOpportunityUsecase(commerce.NewMemoryOpportunityRepo()),
		Sourcing:    commerce.NewSourcingUsecase(commerce.NewMemorySupplierRepo()),
	}
}

// Configure replaces the singleton with Ent-backed (or injected) repos.
// Safe to call once at admin boot; subsequent calls replace the stack but
// preserve Graph / webhook / notify side-hooks registered via Configure* helpers
// (or deps) so admin HTTP Configure cannot wipe WireGraphTaskRuntime hooks.
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
	if deps.OutboundWebhook != nil {
		outbound = deps.OutboundWebhook
	}
	if deps.ReviewNotify != nil {
		notifyFn = deps.ReviewNotify
	}
	if deps.GraphResume != nil {
		resumeFn = deps.GraphResume
	}
	if deps.GraphTaskComplete != nil {
		completeFn = deps.GraphTaskComplete
	}
	rt := &Runtime{
		Content:     content.NewUsecase(contentRepo),
		Review:      review.NewUsecase(reviewRepo),
		Publish:     pubUC,
		Opportunity: commerce.NewOpportunityUsecase(oppRepo),
		Sourcing:    commerce.NewSourcingUsecase(supRepo),
	}
	applySideHooksLocked(rt)
	inst = rt
	return inst
}

// ResetForTest clears the singleton and side-hooks (tests only).
func ResetForTest() {
	mu.Lock()
	defer mu.Unlock()
	inst = nil
	outbound = nil
	notifyFn = nil
	resumeFn = nil
	completeFn = nil
	resetSearcherForTest()
	resetChannelEnqueueForTest()
}

// ConfigureGraphHooks attaches Graph resume callbacks without replacing repos.
// Survives a later Configure() that only swaps Ent repos.
func ConfigureGraphHooks(resume GraphResumeFn, complete GraphTaskCompleteFn) {
	mu.Lock()
	defer mu.Unlock()
	if resume != nil {
		resumeFn = resume
	}
	if complete != nil {
		completeFn = complete
	}
	if inst == nil {
		inst = newMemoryRuntime()
	}
	applySideHooksLocked(inst)
}

// ConfigureOutboundWebhook wires Opportunity Create / theme.digest to the
// real biz.WebhookDispatcher (NovaTrader intel only — no trading).
func ConfigureOutboundWebhook(d *biz.WebhookDispatcher) {
	mu.Lock()
	defer mu.Unlock()
	outbound = d
	if inst == nil {
		inst = newMemoryRuntime()
	}
	applySideHooksLocked(inst)
}

// ConfigureReviewNotify sets the pending-review IM/HTTP notify hook (noop-safe).
func ConfigureReviewNotify(fn review.NotifyFn) {
	mu.Lock()
	defer mu.Unlock()
	notifyFn = fn
	if inst == nil {
		inst = newMemoryRuntime()
	}
	applySideHooksLocked(inst)
}

func applySideHooksLocked(rt *Runtime) {
	if rt == nil {
		return
	}
	rt.GraphResume = resumeFn
	rt.GraphTaskComplete = completeFn
	if notifyFn != nil && rt.Review != nil {
		rt.Review.SetNotify(notifyFn)
	}
	if rt.Opportunity != nil {
		disp := outbound
		rt.Opportunity.SetWebhook(func(eventType string, payload map[string]any) error {
			mu.Lock()
			if inst != nil {
				inst.WebhookLog = append(inst.WebhookLog, map[string]any{"type": eventType, "payload": payload})
			}
			mu.Unlock()
			if disp == nil {
				return nil
			}
			runID := ""
			sessionID := "mvp-opportunity"
			status := "created"
			if eventType == commerce.EventThemeDigest {
				sessionID = "mvp-theme-digest"
				status = "digest"
			}
			if opp, ok := payload["opportunity"]; ok {
				switch v := opp.(type) {
				case commerce.Opportunity:
					runID = v.ID
				case *commerce.Opportunity:
					if v != nil {
						runID = v.ID
					}
				case map[string]any:
					if id, ok := v["ID"].(string); ok {
						runID = id
					} else if id, ok := v["id"].(string); ok {
						runID = id
					}
				}
			}
			disp.Dispatch(context.Background(), eventType, runID, sessionID, status, payload)
			return nil
		})
	}
}

// PackageFilePath resolves an allowlisted file under a packaged job's PackageDir.
// Honest semantics: packaged ≠ posted_to_platform; this only exports local files.
func PackageFilePath(job publish.Job, name string) (string, error) {
	name = filepath.Base(strings.TrimSpace(name))
	switch name {
	case "manifest.json", "body.md":
	default:
		return "", fmt.Errorf("mvp: package file %q not allowed (only manifest.json/body.md)", name)
	}
	if strings.TrimSpace(job.PackageDir) == "" {
		return "", fmt.Errorf("mvp: job has no package_dir (status=%s)", job.Status)
	}
	cleanDir := filepath.Clean(job.PackageDir)
	full := filepath.Join(cleanDir, name)
	if filepath.Dir(full) != cleanDir {
		return "", fmt.Errorf("mvp: path escape blocked")
	}
	return full, nil
}
