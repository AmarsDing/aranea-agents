package memory

import (
	"context"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/jsonutil"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
	trpcfunction "trpc.group/trpc-go/trpc-agent-go/tool/function"
)

// FactReader is the narrow read interface needed by edit tools.
// biz.L3FactReader satisfies this implicitly.
type FactReader interface {
	GetFactRowsByIDs(ctx context.Context, factIDs []string) ([][]byte, error)
}

// FactWriter is the narrow write interface needed by edit tools.
// biz.L3FactWriter satisfies this implicitly.
type FactWriter interface {
	UpsertFactRow(ctx context.Context, in biz.FactUpsert) ([]byte, error)
}

// --- Context keys for dependency injection ---

type l3FactReaderKey struct{}
type l3FactWriterKey struct{}
type factIndexSyncerKey struct{}
type actionLogWriterKey struct{}
type editAgentIDKey struct{}
type editUserIDKey struct{}

// WithL3FactReader injects FactReader into context for tool execution.
func WithL3FactReader(ctx context.Context, r FactReader) context.Context {
	return context.WithValue(ctx, l3FactReaderKey{}, r)
}

// WithL3FactWriter injects FactWriter into context for tool execution.
func WithL3FactWriter(ctx context.Context, w FactWriter) context.Context {
	return context.WithValue(ctx, l3FactWriterKey{}, w)
}

// WithFactIndexSyncer injects MemoryFactIndexSyncer into context for tool execution.
func WithFactIndexSyncer(ctx context.Context, s biz.MemoryFactIndexSyncer) context.Context {
	return context.WithValue(ctx, factIndexSyncerKey{}, s)
}

// WithActionLogWriter injects MemoryActionLogWriter into context for tool execution.
func WithActionLogWriter(ctx context.Context, w biz.MemoryActionLogWriter) context.Context {
	return context.WithValue(ctx, actionLogWriterKey{}, w)
}

// WithEditAgentID injects agent_id into context for tool execution.
func WithEditAgentID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, editAgentIDKey{}, id)
}

// WithEditUserID injects user_id into context for tool execution.
func WithEditUserID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, editUserIDKey{}, id)
}

// L3FactReaderFromCtx extracts FactReader from context.
func L3FactReaderFromCtx(ctx context.Context) FactReader {
	v, _ := ctx.Value(l3FactReaderKey{}).(FactReader)
	return v
}

// L3FactWriterFromCtx extracts FactWriter from context.
func L3FactWriterFromCtx(ctx context.Context) FactWriter {
	v, _ := ctx.Value(l3FactWriterKey{}).(FactWriter)
	return v
}

// FactIndexSyncerFromCtx extracts MemoryFactIndexSyncer from context.
func FactIndexSyncerFromCtx(ctx context.Context) biz.MemoryFactIndexSyncer {
	v, _ := ctx.Value(factIndexSyncerKey{}).(biz.MemoryFactIndexSyncer)
	return v
}

// ActionLogWriterFromCtx extracts MemoryActionLogWriter from context.
func ActionLogWriterFromCtx(ctx context.Context) biz.MemoryActionLogWriter {
	v, _ := ctx.Value(actionLogWriterKey{}).(biz.MemoryActionLogWriter)
	return v
}

// EditAgentIDFromCtx extracts agent_id from context.
func EditAgentIDFromCtx(ctx context.Context) string {
	v, _ := ctx.Value(editAgentIDKey{}).(string)
	return v
}

// EditUserIDFromCtx extracts user_id from context.
func EditUserIDFromCtx(ctx context.Context) string {
	v, _ := ctx.Value(editUserIDKey{}).(string)
	return v
}

// --- Shared helpers ---

// fetchFact retrieves a fact row by ID and returns its parsed map and statement.
func fetchFact(ctx context.Context, reader FactReader, factID string) (map[string]any, string, error) {
	rows, err := reader.GetFactRowsByIDs(ctx, []string{factID})
	if err != nil {
		return nil, "", err
	}
	if len(rows) == 0 {
		return nil, "", apierror.NotFound(apierror.DomainMemory, fmt.Sprintf("fact %q not found", factID))
	}
	m, err := jsonutil.ParseMap(rows[0])
	if err != nil {
		return nil, "", fmt.Errorf("parse fact row: %w", err)
	}
	statement := jsonutil.IfaceStr(m, "statement")
	if statement == "" {
		return nil, "", apierror.BadRequest(apierror.DomainMemory, "fact has empty statement")
	}
	return m, statement, nil
}

// applyEdit persists the edited fact, re-syncs the index, and writes an action log entry.
// Index sync failure is non-fatal (best-effort) — the edit itself is already persisted.
func applyEdit(ctx context.Context, factMap map[string]any, newStatement, action, reason string) error {
	writer := L3FactWriterFromCtx(ctx)
	if writer == nil {
		return apierror.Internal(apierror.DomainMemory, "L3FactWriter not available in context")
	}

	factID := jsonutil.IfaceStr(factMap, "id")
	in := biz.FactUpsert{
		ID:          factID,
		ScopeType:   jsonutil.IfaceStr(factMap, "scope_type"),
		ScopeID:     jsonutil.IfaceStr(factMap, "scope_id"),
		WorkspaceID: jsonutil.IfaceStr(factMap, "workspace_id"),
		UserID:      jsonutil.IfaceStr(factMap, "user_id"),
		AgentID:     jsonutil.IfaceStr(factMap, "agent_id"),
		Statement:   newStatement,
	}
	raw, err := writer.UpsertFactRow(ctx, in)
	if err != nil {
		return err
	}

	// Best-effort index re-sync — failure does not block the edit.
	if syncer := FactIndexSyncerFromCtx(ctx); syncer != nil {
		_ = syncer.SyncFactIndexFromRow(ctx, raw)
	}

	// Best-effort action log — failure does not block the edit.
	if logWriter := ActionLogWriterFromCtx(ctx); logWriter != nil {
		_ = logWriter.WriteMemoryActionLog(ctx, biz.MemoryPolicyRecord{
			Action:        action,
			TargetKind:    "fact",
			TargetID:      factID,
			Reason:        reason,
			PolicyVersion: "agent_edit_v1",
			MetadataJSON:  fmt.Sprintf(`{"old":"%s","new":"%s"}`, truncate(jsonutil.IfaceStr(factMap, "statement"), 200), truncate(newStatement, 200)),
		})
	}

	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// --- memory_replace ---

// ReplaceInput is the input for memory_replace.
type ReplaceInput struct {
	MemoryID string `json:"memory_id" jsonschema:"description=The ID of the memory fact to edit,required"`
	OldText  string `json:"old_text" jsonschema:"description=The text fragment to find and replace,required"`
	NewText  string `json:"new_text" jsonschema:"description=The replacement text,required"`
}

// ReplaceOutput is the output for memory_replace.
type ReplaceOutput struct {
	Success    bool   `json:"success"`
	OldContent string `json:"old_content"`
	NewContent string `json:"new_content"`
	Message    string `json:"message"`
}

func replaceExecute(ctx context.Context, input ReplaceInput) (ReplaceOutput, error) {
	reader := L3FactReaderFromCtx(ctx)
	if reader == nil {
		return ReplaceOutput{}, apierror.Internal(apierror.DomainMemory, "L3FactReader not available in context")
	}

	factMap, oldStatement, err := fetchFact(ctx, reader, strings.TrimSpace(input.MemoryID))
	if err != nil {
		return ReplaceOutput{}, err
	}

	oldText := strings.TrimSpace(input.OldText)
	if oldText == "" {
		return ReplaceOutput{}, apierror.BadRequest(apierror.DomainMemory, "old_text must not be empty")
	}
	if !strings.Contains(oldStatement, oldText) {
		return ReplaceOutput{}, apierror.BadRequest(apierror.DomainMemory, fmt.Sprintf("old_text %q not found in memory %q", oldText, input.MemoryID))
	}

	newStatement := strings.Replace(oldStatement, oldText, strings.TrimSpace(input.NewText), 1)
	if err := applyEdit(ctx, factMap, newStatement, "replace", fmt.Sprintf("replace %q with %q", oldText, input.NewText)); err != nil {
		return ReplaceOutput{}, err
	}

	return ReplaceOutput{
		Success:    true,
		OldContent: oldStatement,
		NewContent: newStatement,
		Message:    "Memory updated successfully",
	}, nil
}

// NewReplaceTool creates the memory_replace tool.
func NewReplaceTool() trpctool.Tool {
	return trpcfunction.NewFunctionTool(replaceExecute,
		trpcfunction.WithName("memory_replace"),
		trpcfunction.WithDescription("Find and replace a specific text fragment within an existing memory fact. Use this to precisely update part of a memory without rewriting the entire content."),
	)
}

// --- memory_rethink ---

// RethinkInput is the input for memory_rethink.
type RethinkInput struct {
	MemoryID   string `json:"memory_id" jsonschema:"description=The ID of the memory fact to rewrite,required"`
	NewContent string `json:"new_content" jsonschema:"description=The complete new content for the memory,required"`
	Reason     string `json:"reason" jsonschema:"description=The reason for rewriting this memory (used for provenance tracking),required"`
}

// RethinkOutput is the output for memory_rethink.
type RethinkOutput struct {
	Success    bool   `json:"success"`
	OldContent string `json:"old_content"`
	NewContent string `json:"new_content"`
	Message    string `json:"message"`
}

func rethinkExecute(ctx context.Context, input RethinkInput) (RethinkOutput, error) {
	reader := L3FactReaderFromCtx(ctx)
	if reader == nil {
		return RethinkOutput{}, apierror.Internal(apierror.DomainMemory, "L3FactReader not available in context")
	}

	factMap, oldStatement, err := fetchFact(ctx, reader, strings.TrimSpace(input.MemoryID))
	if err != nil {
		return RethinkOutput{}, err
	}

	newContent := strings.TrimSpace(input.NewContent)
	if newContent == "" {
		return RethinkOutput{}, apierror.BadRequest(apierror.DomainMemory, "new_content must not be empty")
	}

	if err := applyEdit(ctx, factMap, newContent, "rethink", strings.TrimSpace(input.Reason)); err != nil {
		return RethinkOutput{}, err
	}

	return RethinkOutput{
		Success:    true,
		OldContent: oldStatement,
		NewContent: newContent,
		Message:    "Memory rewritten successfully",
	}, nil
}

// NewRethinkTool creates the memory_rethink tool.
func NewRethinkTool() trpctool.Tool {
	return trpcfunction.NewFunctionTool(rethinkExecute,
		trpcfunction.WithName("memory_rethink"),
		trpcfunction.WithDescription("Completely rewrite an existing memory fact with new content. Use this when the memory needs deep restructuring based on new information. The reason is recorded for provenance."),
	)
}

// --- memory_insert ---

// InsertInput is the input for memory_insert.
type InsertInput struct {
	MemoryID    string `json:"memory_id" jsonschema:"description=The ID of the memory fact to insert into,required"`
	AfterText   string `json:"after_text" jsonschema:"description=The text after which to insert the new content,required"`
	InsertText  string `json:"insert_text" jsonschema:"description=The text to insert,required"`
}

// InsertOutput is the output for memory_insert.
type InsertOutput struct {
	Success    bool   `json:"success"`
	OldContent string `json:"old_content"`
	NewContent string `json:"new_content"`
	Message    string `json:"message"`
}

func insertExecute(ctx context.Context, input InsertInput) (InsertOutput, error) {
	reader := L3FactReaderFromCtx(ctx)
	if reader == nil {
		return InsertOutput{}, apierror.Internal(apierror.DomainMemory, "L3FactReader not available in context")
	}

	factMap, oldStatement, err := fetchFact(ctx, reader, strings.TrimSpace(input.MemoryID))
	if err != nil {
		return InsertOutput{}, err
	}

	afterText := strings.TrimSpace(input.AfterText)
	if afterText == "" {
		return InsertOutput{}, apierror.BadRequest(apierror.DomainMemory, "after_text must not be empty")
	}

	idx := strings.Index(oldStatement, afterText)
	if idx < 0 {
		return InsertOutput{}, apierror.BadRequest(apierror.DomainMemory, fmt.Sprintf("after_text %q not found in memory %q", afterText, input.MemoryID))
	}

	insertPos := idx + len(afterText)
	newStatement := oldStatement[:insertPos] + strings.TrimSpace(input.InsertText) + oldStatement[insertPos:]

	if err := applyEdit(ctx, factMap, newStatement, "insert", fmt.Sprintf("insert after %q", afterText)); err != nil {
		return InsertOutput{}, err
	}

	return InsertOutput{
		Success:    true,
		OldContent: oldStatement,
		NewContent: newStatement,
		Message:    "Content inserted successfully",
	}, nil
}

// NewInsertTool creates the memory_insert tool.
func NewInsertTool() trpctool.Tool {
	return trpcfunction.NewFunctionTool(insertExecute,
		trpcfunction.WithName("memory_insert"),
		trpcfunction.WithDescription("Insert new text at a specific position within an existing memory fact. The new text is inserted immediately after the specified after_text fragment."),
	)
}

// --- AdvancedTools ---

// AdvancedTools returns the three agent self-edit memory tools.
// These tools allow agents to precisely edit existing L3 semantic facts
// (replace fragments, rewrite entirely, or insert new content at specific positions).
func AdvancedTools() []trpctool.Tool {
	return []trpctool.Tool{
		NewReplaceTool(),
		NewRethinkTool(),
		NewInsertTool(),
	}
}
