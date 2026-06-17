package trpc

import (
	"context"
	"errors"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/skill"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	trpcskill "trpc.group/trpc-go/trpc-agent-go/skill"
)

// ErrSkillNotFound is returned by DBStore methods when the requested skill
// does not exist in the backing store.
var ErrSkillNotFound = errors.New("skill not found")

// DBStore defines the persistence contract for skill data.
// This is the project-internal equivalent of a framework dbrepository.Store.
type DBStore interface {
	// ListSummaries returns all skill summaries visible to the runtime.
	ListSummaries(ctx context.Context) ([]trpcskill.Summary, error)
	// GetByName returns the full skill identified by name (slug).
	GetByName(ctx context.Context, name string) (*trpcskill.Skill, error)
	// GetPathByName returns the on-disk storage directory for the named skill.
	GetPathByName(ctx context.Context, name string) (string, error)
}

// DBStoreAdapter bridges the project's biz skill layer to the DBStore interface.
// It converts biz-level skill types to framework-level skill types, enabling
// DBRepository to consume skill data from the project's data layer.
type DBStoreAdapter struct {
	runtimeReader skill.SkillRuntimeReader
	lookupReader  skill.SkillLookupReader
	lg            loggateway.Logger
}

// Compile-time interface compliance check.
var _ DBStore = (*DBStoreAdapter)(nil)

// NewDBStoreAdapter creates a DBStoreAdapter from the project's biz skill readers.
func NewDBStoreAdapter(
	runtimeReader skill.SkillRuntimeReader,
	lookupReader skill.SkillLookupReader,
	lg loggateway.Logger,
) *DBStoreAdapter {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &DBStoreAdapter{
		runtimeReader: runtimeReader,
		lookupReader:  lookupReader,
		lg:            lg,
	}
}

// ListSummaries returns all enabled+published skill summaries from the DB.
func (a *DBStoreAdapter) ListSummaries(ctx context.Context) ([]trpcskill.Summary, error) {
	candidates, err := a.runtimeReader.ListEnabledPublishedSkillCandidates(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]trpcskill.Summary, 0, len(candidates))
	for _, c := range candidates {
		slug := biz.NormalizeSkillSlug(c.Slug)
		if slug == "" {
			continue
		}
		// Summary.Name is the canonical handle (slug) used by trpcskill.Get.
		// Display name is preserved via the Description prefix so prompts
		// retain human-readable context.
		desc := strings.TrimSpace(c.Description)
		if display := strings.TrimSpace(c.Name); display != "" && !strings.EqualFold(display, slug) {
			if desc == "" {
				desc = display
			} else {
				desc = display + " — " + desc
			}
		}
		out = append(out, trpcskill.Summary{
			Name:        slug,
			Description: desc,
		})
	}
	return out, nil
}

// GetByName returns the full skill identified by name (slug).
func (a *DBStoreAdapter) GetByName(ctx context.Context, name string) (*trpcskill.Skill, error) {
	slug := biz.NormalizeSkillSlug(name)
	if slug == "" {
		return nil, ErrSkillNotFound
	}
	sk, err := a.lookupReader.GetSkillBySkillKey(ctx, slug)
	if err != nil {
		if isNotFound(err) {
			return nil, ErrSkillNotFound
		}
		return nil, err
	}
	body, err := a.lookupReader.GetLatestSkillMarkdown(ctx, sk.ID)
	if err != nil {
		a.lg.Warn("DBStoreAdapter: GetLatestSkillMarkdown failed",
			loggateway.StepID("skill.db_store_adapter"),
			loggateway.Str("slug", slug),
			loggateway.Err(err))
		body = "" // Graceful degradation: return skill with empty body
	}
	return &trpcskill.Skill{
		Summary: trpcskill.Summary{
			Name:        slug,
			Description: sk.Description,
		},
		Body: body,
	}, nil
}

// GetPathByName returns the on-disk storage directory for the named skill.
func (a *DBStoreAdapter) GetPathByName(ctx context.Context, name string) (string, error) {
	slug := biz.NormalizeSkillSlug(name)
	if slug == "" {
		return "", ErrSkillNotFound
	}
	sk, err := a.lookupReader.GetSkillBySkillKey(ctx, slug)
	if err != nil {
		if isNotFound(err) {
			return "", ErrSkillNotFound
		}
		return "", err
	}
	dir, err := a.lookupReader.GetSkillStorageDir(ctx, sk.ID)
	if err != nil {
		// DB-only skills may not have a storage directory — return empty string.
		return "", nil
	}
	return dir, nil
}

// isNotFound checks if the error indicates a not-found condition.
func isNotFound(err error) bool {
	return apierror.IsCode(err, apierror.CodeNotFound)
}
