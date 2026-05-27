package trpc

import (
	"context"

	trpcskill "trpc.group/trpc-go/trpc-agent-go/skill"
)

// NewFilteredRepository wraps base with a slug allow-list. allowedSlugs are matched
// against the canonical slug derived from Summary.Name. TPM-P1-06 — both sides are
// canonicalized so that "My Skill" / "my-skill" / "MY_SKILL" all collapse identically.
func NewFilteredRepository(base trpcskill.Repository, allowedSlugs []string) trpcskill.ContextRepository {
	allow := make(map[string]struct{}, len(allowedSlugs))
	for _, s := range allowedSlugs {
		c := canonicalSlug(s)
		if c != "" {
			allow[c] = struct{}{}
		}
	}
	filter := func(_ context.Context, summary trpcskill.Summary) bool {
		if len(allow) == 0 {
			return true
		}
		_, ok := allow[canonicalSlug(summary.Name)]
		return ok
	}
	return trpcskill.NewFilteredRepository(base, filter)
}
