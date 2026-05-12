package trpc

import (
	"context"
	"strings"

	trpcskill "trpc.group/trpc-go/trpc-agent-go/skill"
)

func NewFilteredRepository(base trpcskill.Repository, allowedSlugs []string) trpcskill.ContextRepository {
	allowSet := sliceToSet(allowedSlugs)
	filter := func(_ context.Context, summary trpcskill.Summary) bool {
		if len(allowSet) == 0 {
			return true
		}
		name := strings.TrimSpace(strings.ToLower(summary.Name))
		return allowSet[name]
	}
	return trpcskill.NewFilteredRepository(base, filter)
}

func sliceToSet(slugs []string) map[string]bool {
	m := map[string]bool{}
	for _, s := range slugs {
		s = strings.TrimSpace(strings.ToLower(s))
		if s != "" {
			m[s] = true
		}
	}
	return m
}
