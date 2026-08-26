// Package memory provides the standard memory tools (add/update/load/search/delete)
// as a registry-friendly helper.  The tools themselves are stateless at build time;
// they obtain the MemoryService from the invocation context at call time, so the
// runner must be configured with WithMemoryService before running any turn.
package memory

import (
	trpcmemtool "trpc.group/trpc-go/trpc-agent-go/memory/tool"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// DefaultTools returns the five standard memory tools.
// The "clear" tool is intentionally omitted from the default set to avoid
// accidental bulk deletion by the model.
// memory_search / memory_load are wrapped (search_note.go) so zero-hit
// responses carry the R3 guidance note; add/update/delete pass through
// unchanged.
func DefaultTools() []trpctool.Tool {
	return []trpctool.Tool{
		trpcmemtool.NewAddTool(),
		trpcmemtool.NewUpdateTool(),
		&emptyNoteLoadTool{inner: trpcmemtool.NewLoadTool()},
		&emptyNoteSearchTool{inner: trpcmemtool.NewSearchTool()},
		trpcmemtool.NewDeleteTool(),
	}
}
