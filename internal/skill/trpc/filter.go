package trpc

import (
	"context"
	"strings"

	"aranea-agents/pkg/strutil"

	trpcskill "trpc.group/trpc-go/trpc-agent-go/skill"
)

func NewFilteredRepository(base trpcskill.Repository, allowedSlugs []string) trpcskill.ContextRepository {
	allowSet := strutil.SliceToSet(allowedSlugs)
	filter := func(_ context.Context, summary trpcskill.Summary) bool {
		if len(allowSet) == 0 {
			return true
		}
		name := strings.TrimSpace(strings.ToLower(summary.Name))
		return allowSet[name]
	}
	return trpcskill.NewFilteredRepository(base, filter)
}
