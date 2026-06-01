package tools

import (
	"strings"
	"sync"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

func buildToolGroups() map[string][]string {
	groups := make(map[string][]string)
	for _, reg := range Registry() {
		if reg.Group != "" {
			groups[reg.Group] = append(groups[reg.Group], reg.Name)
		}
	}
	return groups
}

var toolGroupsCache map[string][]string
var toolGroupsOnce sync.Once

func getToolGroups() map[string][]string {
	toolGroupsOnce.Do(func() {
		toolGroupsCache = buildToolGroups()
	})
	return toolGroupsCache
}

func ApplyDisambiguationHints(tools []trpctool.Tool) {
	for _, t := range tools {
		decl := t.Declaration()
		if decl == nil {
			continue
		}
		for _, reg := range Registry() {
			if reg.Name != decl.Name {
				continue
			}
			if len(reg.Examples) > 0 {
				var sb strings.Builder
				sb.WriteString(decl.Description)
				sb.WriteString("\n\nExamples of when to use this tool:")
				for i, ex := range reg.Examples {
					if i >= 3 {
						break
					}
					sb.WriteString("\n- When user asks: \"")
					sb.WriteString(ex.UserQuery)
					sb.WriteString("\"")
					if ex.Explanation != "" {
						sb.WriteString(" (")
						sb.WriteString(ex.Explanation)
						sb.WriteString(")")
					}
				}
				decl.Description = sb.String()
			}
			if reg.Group != "" {
				if peers, ok := getToolGroups()[reg.Group]; ok && len(peers) > 1 {
					decl.Description += "\n\nNote: This tool is in the \"" + reg.Group + "\" group. Alternatives: " + strings.Join(filterNames(peers, decl.Name), ", ") + "."
				}
			}
			break
		}
	}
}

func filterNames(names []string, exclude string) []string {
	var result []string
	for _, n := range names {
		if n != exclude {
			result = append(result, n)
		}
	}
	return result
}
