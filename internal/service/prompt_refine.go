package service

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	airefinev1 "aranea-agents/api/kratos/ai_refine/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/auth"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

// Rate-limit reasons (B-6). Front-end branches on these distinct codes.
const (
	refineReasonRateLimit     = "REFINE_RATE_LIMIT"
	refineReasonRateLimitUser = "REFINE_RATE_LIMIT_USER"
)

// AIRefineService implements the AIRefineService proto contract (PGO-3-SVC-01,
// RL-12 fix): the /v1/ai/refine endpoint is now proto-defined and registered via
// the generated RegisterAIRefineServiceHTTPServer, not a hand-rolled route.
//
// The rate limiter lives in this layer because it needs the HTTP auth user ID
// — a transport-layer concern, not business policy.
type AIRefineService struct {
	airefinev1.UnimplementedAIRefineServiceServer
	refiner   *biz.PromptRefiner
	rateLimit *refineRateLimiter
}

// NewAIRefineService wires the service.
func NewAIRefineService(refiner *biz.PromptRefiner) *AIRefineService {
	return &AIRefineService{
		refiner: refiner,
		rateLimit: newRefineRateLimiter(
			20,            // global QPS burst
			5*time.Minute, // per-user window
			10,            // max per user per window
		),
	}
}

// scopeMap converts the proto enum into the biz FieldScope string. Unknown
// values map to the empty scope, which causes PromptRefiner.Refine to return
// REFINE_UNKNOWN_SCOPE — preserving the original validation behaviour.
var scopeMap = map[airefinev1.RefineScope]biz.FieldScope{
	airefinev1.RefineScope_REFINE_SCOPE_CATEGORY_INDUSTRY: biz.ScopeCategoryIndustry,
	airefinev1.RefineScope_REFINE_SCOPE_CATEGORY_DEPT:     biz.ScopeCategoryDepartment,
	airefinev1.RefineScope_REFINE_SCOPE_CATEGORY_POSITION: biz.ScopeCategoryPosition,
	airefinev1.RefineScope_REFINE_SCOPE_AGENT_DESCRIPTION: biz.ScopeAgentDescription,
	airefinev1.RefineScope_REFINE_SCOPE_AGENT_FILE:        biz.ScopeAgentFile,
	// PGO-4-PROTO-01: SPEC_EXTRACT enables markdown → YAML org spec extraction via CLI.
	airefinev1.RefineScope_REFINE_SCOPE_SPEC_EXTRACT: biz.ScopeSpecExtract,
}

// Refine implements airefinev1.AIRefineServiceHTTPServer.
func (s *AIRefineService) Refine(ctx context.Context, req *airefinev1.RefineRequest) (*airefinev1.RefineResponse, error) {
	userID := userIDFromCtx(ctx)
	if err := s.rateLimit.allow(ctx, userID); err != nil {
		return nil, err
	}

	scope := scopeMap[req.GetScope()]
	out, err := s.refiner.Refine(ctx, biz.RefineRequest{
		Scope:        scope,
		ResourceID:   req.GetResourceId(),
		FileName:     req.GetFileName(),
		OriginalText: req.GetOriginalText(),
		UserHint:     req.GetUserHint(),
		TargetMode:   req.GetTargetMode(),
	})
	if err != nil {
		return nil, err
	}

	return &airefinev1.RefineResponse{
		Refined:      out.Refined,
		Diff:         out.Diff,
		TokensBefore: int32(out.TokensBefore),
		TokensAfter:  int32(out.TokensAfter),
		Provider:     out.Provider,
		Model:        out.Model,
		Source:       string(out.ModelSource),
	}, nil
}

// userIDFromCtx returns the auth user ID as a string, or "" if anonymous.
func userIDFromCtx(ctx context.Context) string {
	if a, ok := auth.FromContext(ctx); ok && a != nil {
		return fmt.Sprintf("%d", a.UserID)
	}
	return ""
}

// ─── Rate limiter ─────────────────────────────────────────────────────────────

// refineRateLimiter enforces transport-layer rate limits. PGO-3-SVC-03.
// Kept in the service layer because it needs the HTTP user ID from auth context.
type refineRateLimiter struct {
	globalBurst     int
	perUserWindow   time.Duration
	perUserMax      int
	mu              sync.Mutex
	globalLastReset time.Time
	globalCount     int
	perUserCounts   map[string]*userWindow
}

type userWindow struct {
	reset time.Time
	count int
}

func newRefineRateLimiter(globalBurst int, perUserWindow time.Duration, perUserMax int) *refineRateLimiter {
	return &refineRateLimiter{
		globalBurst:     globalBurst,
		perUserWindow:   perUserWindow,
		perUserMax:      perUserMax,
		globalLastReset: time.Now(),
		perUserCounts:   make(map[string]*userWindow),
	}
}

func (r *refineRateLimiter) allow(_ context.Context, userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	if now.Sub(r.globalLastReset) >= time.Second {
		r.globalCount = 0
		r.globalLastReset = now
	}
	r.globalCount++
	if r.globalCount > r.globalBurst {
		return kerrors.New(http.StatusTooManyRequests, refineReasonRateLimit, "system refine rate limit exceeded; retry in 1s")
	}

	if userID != "" {
		uw, ok := r.perUserCounts[userID]
		if !ok || now.After(uw.reset) {
			uw = &userWindow{reset: now.Add(r.perUserWindow)}
			r.perUserCounts[userID] = uw
		}
		uw.count++
		if uw.count > r.perUserMax {
			return kerrors.New(http.StatusTooManyRequests, refineReasonRateLimitUser, "personal refine limit reached; try again later")
		}
	}
	return nil
}

// Compile-time assertion that AIRefineService implements the HTTP server.
var _ airefinev1.AIRefineServiceHTTPServer = (*AIRefineService)(nil)
