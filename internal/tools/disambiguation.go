package tools

import (
	"context"
	"fmt"
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
	for i, t := range tools {
		decl := t.Declaration()
		if decl == nil {
			continue
		}
		for _, reg := range Registry() {
			if reg.Name != decl.Name {
				continue
			}
			// Clone Declaration to avoid mutating shared objects.
			// TECH-DEBT: If Declaration gains additional reference-type fields
			// beyond InputSchema, update this deep-copy logic accordingly.
			clone := *decl
			if decl.InputSchema != nil {
				schemaCopy := *decl.InputSchema
				clone.InputSchema = &schemaCopy
			}
			if len(reg.Examples) > 0 {
				var sb strings.Builder
				sb.WriteString(clone.Description)
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
				clone.Description = sb.String()
			}
			if reg.Group != "" {
				if peers, ok := getToolGroups()[reg.Group]; ok && len(peers) > 1 {
					clone.Description += "\n\nNote: This tool is in the \"" + reg.Group + "\" group. Alternatives: " + strings.Join(filterNames(peers, clone.Name), ", ") + "."
				}
			}
			// Wrap the tool with a decorator that returns the cloned Declaration
			// instead of mutating the shared Declaration object.
			tools[i] = &disambiguatedTool{inner: t, decl: &clone}
			break
		}
	}
}

// disambiguatedTool wraps a Tool to return a modified Declaration without
// mutating the original shared Declaration object.
type disambiguatedTool struct {
	inner trpctool.Tool
	decl  *trpctool.Declaration
}

func (d *disambiguatedTool) Declaration() *trpctool.Declaration {
	return d.decl
}

func (d *disambiguatedTool) Call(ctx context.Context, jsonArgs []byte) (any, error) {
	if ct, ok := d.inner.(trpctool.CallableTool); ok {
		return ct.Call(ctx, jsonArgs)
	}
	return nil, fmt.Errorf("tool %q is not callable", d.decl.Name)
}

func (d *disambiguatedTool) StreamableCall(ctx context.Context, jsonArgs []byte) (*trpctool.StreamReader, error) {
	if st, ok := d.inner.(trpctool.StreamableTool); ok {
		return st.StreamableCall(ctx, jsonArgs)
	}
	return nil, fmt.Errorf("tool %q is not streamable", d.decl.Name)
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
