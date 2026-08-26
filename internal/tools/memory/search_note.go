// Empty-hit guidance for the memory read tools (79-runtime-governance R3
// 顺手项): when memory_search / memory_load return zero rows the model must
// not conclude "the fact does not exist" — zero hits only means the query
// missed. The wrapper delegates to the upstream tool untouched and only
// annotates the zero-hit response with a guidance note.
package memory

import (
	"context"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
	trpcmemtool "trpc.group/trpc-go/trpc-agent-go/memory/tool"
)

// memoryReadEmptyNote is the R3 guidance text pinned by the requirement
// («0 结果不代表未发生/未收录，可换罕见词或扩大范围») plus the honesty rule
// shared with knowledge_search's empty note (never invent details).
const memoryReadEmptyNote = "0 结果不代表未发生/未收录，可换罕见词或扩大范围。" +
	"Zero matching memories — the fact may exist under different keywords (try rarer terms), " +
	"a wider time/limit range, or it may simply not be recorded yet. " +
	"Do not invent names, dates, IDs, or preferences; say the memory has no matching record."

// searchMemoryResponseWithNote mirrors the upstream search response with an
// added guidance note (serialized flat: query/results/count/note).
type searchMemoryResponseWithNote struct {
	trpcmemtool.SearchMemoryResponse
	Note string `json:"note"`
}

// loadMemoryResponseWithNote mirrors the upstream load response with an
// added guidance note (limit/results/count/note).
type loadMemoryResponseWithNote struct {
	trpcmemtool.LoadMemoryResponse
	Note string `json:"note"`
}

// emptyNoteSearchTool wraps the upstream memory_search tool: full delegate,
// annotating only zero-hit responses.
type emptyNoteSearchTool struct {
	inner trpctool.CallableTool
}

func (t *emptyNoteSearchTool) Declaration() *trpctool.Declaration {
	return t.inner.Declaration()
}

func (t *emptyNoteSearchTool) Call(ctx context.Context, jsonArgs []byte) (any, error) {
	res, err := t.inner.Call(ctx, jsonArgs)
	if err != nil {
		return res, err
	}
	if resp, ok := res.(*trpcmemtool.SearchMemoryResponse); ok && resp.Count == 0 {
		return &searchMemoryResponseWithNote{
			SearchMemoryResponse: *resp,
			Note:                 memoryReadEmptyNote,
		}, nil
	}
	return res, nil
}

// emptyNoteLoadTool wraps the upstream memory_load tool the same way.
type emptyNoteLoadTool struct {
	inner trpctool.CallableTool
}

func (t *emptyNoteLoadTool) Declaration() *trpctool.Declaration {
	return t.inner.Declaration()
}

func (t *emptyNoteLoadTool) Call(ctx context.Context, jsonArgs []byte) (any, error) {
	res, err := t.inner.Call(ctx, jsonArgs)
	if err != nil {
		return res, err
	}
	if resp, ok := res.(*trpcmemtool.LoadMemoryResponse); ok && resp.Count == 0 {
		return &loadMemoryResponseWithNote{
			LoadMemoryResponse: *resp,
			Note:               memoryReadEmptyNote,
		}, nil
	}
	return res, nil
}
