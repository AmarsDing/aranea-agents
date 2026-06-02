package working_memory

import (
	"context"
	"encoding/json"
	"fmt"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/jsonutil"

	trpcfunction "trpc.group/trpc-go/trpc-agent-go/tool/function"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// --- Context keys for dependency injection ---

type l1WriterKey struct{}
type l1ReaderKey struct{}
type sessionIDKey struct{}
type agentIDKey struct{}

// WithL1Writer injects L1Writer into context for tool execution.
func WithL1Writer(ctx context.Context, w biz.L1Writer) context.Context {
	return context.WithValue(ctx, l1WriterKey{}, w)
}

// WithL1Reader injects L1AdminReader into context for tool execution.
func WithL1Reader(ctx context.Context, r biz.L1AdminReader) context.Context {
	return context.WithValue(ctx, l1ReaderKey{}, r)
}

// WithSessionID injects session_id into context for tool execution.
func WithSessionID(ctx context.Context, sid string) context.Context {
	return context.WithValue(ctx, sessionIDKey{}, sid)
}

// WithAgentID injects agent_id into context for tool execution.
func WithAgentID(ctx context.Context, aid string) context.Context {
	return context.WithValue(ctx, agentIDKey{}, aid)
}

// L1WriterFromCtx extracts L1Writer from context.
func L1WriterFromCtx(ctx context.Context) biz.L1Writer {
	v, _ := ctx.Value(l1WriterKey{}).(biz.L1Writer)
	return v
}

// L1ReaderFromCtx extracts L1AdminReader from context.
func L1ReaderFromCtx(ctx context.Context) biz.L1AdminReader {
	v, _ := ctx.Value(l1ReaderKey{}).(biz.L1AdminReader)
	return v
}

// SessionIDFromCtx extracts session_id from context.
func SessionIDFromCtx(ctx context.Context) string {
	v, _ := ctx.Value(sessionIDKey{}).(string)
	return v
}

// AgentIDFromCtx extracts agent_id from context.
func AgentIDFromCtx(ctx context.Context) string {
	v, _ := ctx.Value(agentIDKey{}).(string)
	return v
}

// --- Helpers ---

// findActiveTaskID finds the active task ID for the given session+agent.
func findActiveTaskID(ctx context.Context, reader biz.L1AdminReader, sessID, agentID string) (string, error) {
	rows, err := reader.ListL1TaskRows(ctx, sessID, agentID, "active", "")
	if err != nil {
		return "", err
	}
	if len(rows) == 0 {
		return "", nil
	}
	m, _ := jsonutil.ParseMap(rows[0])
	return jsonutil.IfaceStr(m, "id"), nil
}

// ensureActiveTask finds or creates an active task for the session+agent.
func ensureActiveTask(ctx context.Context, writer biz.L1Writer, reader biz.L1AdminReader, sessID, agentID string) (string, error) {
	taskID, err := findActiveTaskID(ctx, reader, sessID, agentID)
	if err != nil {
		return "", err
	}
	if taskID != "" {
		return taskID, nil
	}
	// Auto-create a task
	raw, err := writer.StartL1Task(ctx, biz.L1TaskInsert{
		SessionID: sessID,
		AgentID:   agentID,
		TaskKey:   fmt.Sprintf("auto-%s", agentID),
		TaskTitle: "Auto-created working memory task",
		TaskGoal:  "Store and retrieve working memory fields for the current conversation",
	})
	if err != nil {
		return "", err
	}
	m, _ := jsonutil.ParseMap(raw)
	return jsonutil.IfaceStr(m, "id"), nil
}

// --- working_memory.read ---

// ReadInput is the input for working_memory.read.
type ReadInput struct {
	FieldPath string `json:"field_path" jsonschema:"description=The field path to read,required"`
}

// ReadOutput is the output for working_memory.read.
type ReadOutput struct {
	FieldPath string `json:"field_path"`
	Value     string `json:"value"`
	Found     bool   `json:"found"`
}

func readExecute(ctx context.Context, input ReadInput) (ReadOutput, error) {
	writer := L1WriterFromCtx(ctx)
	reader := L1ReaderFromCtx(ctx)
	sessID := SessionIDFromCtx(ctx)
	agentID := AgentIDFromCtx(ctx)
	if writer == nil || reader == nil || sessID == "" {
		return ReadOutput{}, fmt.Errorf("working_memory not available")
	}
	taskID, err := findActiveTaskID(ctx, reader, sessID, agentID)
	if err != nil || taskID == "" {
		return ReadOutput{FieldPath: input.FieldPath, Found: false}, nil
	}
	raw, err := reader.GetL1FieldRow(ctx, taskID, input.FieldPath)
	if err != nil {
		return ReadOutput{FieldPath: input.FieldPath, Found: false}, nil
	}
	m, _ := jsonutil.ParseMap(raw)
	return ReadOutput{
		FieldPath: input.FieldPath,
		Value:     jsonutil.IfaceStr(m, "value_text"),
		Found:     true,
	}, nil
}

// NewReadTool creates the working_memory.read tool.
func NewReadTool() trpctool.Tool {
	return trpcfunction.NewFunctionTool(readExecute,
		trpcfunction.WithName("working_memory.read"),
		trpcfunction.WithDescription("Read a field value from the current working memory task. Use this to retrieve stored context like user preferences, task progress, or intermediate results."),
	)
}

// --- working_memory.list ---

// ListInput is the input for working_memory.list.
type ListInput struct{}

// FieldEntry represents a single field in the list output.
type FieldEntry struct {
	FieldPath string `json:"field_path"`
	Value     string `json:"value"`
	Kind      string `json:"kind"`
	Pinned    bool   `json:"pinned"`
}

// ListOutput is the output for working_memory.list.
type ListOutput struct {
	Fields []FieldEntry `json:"fields"`
}

func listExecute(ctx context.Context, input ListInput) (ListOutput, error) {
	reader := L1ReaderFromCtx(ctx)
	sessID := SessionIDFromCtx(ctx)
	agentID := AgentIDFromCtx(ctx)
	if reader == nil || sessID == "" {
		return ListOutput{}, fmt.Errorf("working_memory not available")
	}
	taskID, err := findActiveTaskID(ctx, reader, sessID, agentID)
	if err != nil || taskID == "" {
		return ListOutput{}, nil
	}
	rows, err := reader.ListL1FieldRows(ctx, taskID, false)
	if err != nil {
		return ListOutput{}, nil
	}
	fields := make([]FieldEntry, 0, len(rows))
	for _, raw := range rows {
		m, _ := jsonutil.ParseMap(raw)
		fields = append(fields, FieldEntry{
			FieldPath: jsonutil.IfaceStr(m, "field_path"),
			Value:     jsonutil.IfaceStr(m, "value_text"),
			Kind:      jsonutil.IfaceStr(m, "field_kind"),
			Pinned:    jsonutil.IfaceBool(m, "pin_to_prompt"),
		})
	}
	return ListOutput{Fields: fields}, nil
}

// NewListTool creates the working_memory.list tool.
func NewListTool() trpctool.Tool {
	return trpcfunction.NewFunctionTool(listExecute,
		trpcfunction.WithName("working_memory.list"),
		trpcfunction.WithDescription("List all fields in the current working memory task. Returns field paths, values, and pin status."),
	)
}

// --- working_memory.write ---

// WriteInput is the input for working_memory.write.
type WriteInput struct {
	FieldPath string `json:"field_path" jsonschema:"description=The field path to write to,required"`
	Value     string `json:"value" jsonschema:"description=The value to store,required"`
	Pinned    bool   `json:"pinned" jsonschema:"description=Whether to pin this field to the prompt"`
}

// WriteOutput is the output for working_memory.write.
type WriteOutput struct {
	FieldPath string `json:"field_path"`
	Value     string `json:"value"`
	Revision  int32  `json:"revision"`
}

func writeExecute(ctx context.Context, input WriteInput) (WriteOutput, error) {
	writer := L1WriterFromCtx(ctx)
	reader := L1ReaderFromCtx(ctx)
	sessID := SessionIDFromCtx(ctx)
	agentID := AgentIDFromCtx(ctx)
	if writer == nil || sessID == "" {
		return WriteOutput{}, fmt.Errorf("working_memory not available")
	}
	taskID, err := ensureActiveTask(ctx, writer, reader, sessID, agentID)
	if err != nil {
		return WriteOutput{}, err
	}
	raw, err := writer.UpsertL1Field(ctx, biz.L1FieldInsert{
		TaskID:      taskID,
		SessionID:   sessID,
		AgentID:     agentID,
		FieldPath:   input.FieldPath,
		ValueText:   input.Value,
		PinToPrompt: input.Pinned,
		Source:      "agent",
		ChangedBy:   "agent",
	})
	if err != nil {
		return WriteOutput{}, err
	}
	m, _ := jsonutil.ParseMap(raw)
	return WriteOutput{
		FieldPath: input.FieldPath,
		Value:     input.Value,
		Revision:  jsonutil.IfaceI32(m, "revision"),
	}, nil
}

// NewWriteTool creates the working_memory.write tool.
func NewWriteTool() trpctool.Tool {
	return trpcfunction.NewFunctionTool(writeExecute,
		trpcfunction.WithName("working_memory.write"),
		trpcfunction.WithDescription("Write a field value to the current working memory task. Creates the task if none exists. Pinned fields are included in the prompt for future turns."),
	)
}

// --- working_memory.patch ---

// PatchField represents a single field in a patch batch.
type PatchField struct {
	FieldPath string `json:"field_path" jsonschema:"description=The field path,required"`
	Value     string `json:"value" jsonschema:"description=The value to store,required"`
	Pinned    bool   `json:"pinned" jsonschema:"description=Whether to pin this field to the prompt"`
}

// PatchInput is the input for working_memory.patch.
type PatchInput struct {
	Fields []PatchField `json:"fields" jsonschema:"description=List of fields to upsert,required"`
}

// PatchOutput is the output for working_memory.patch.
type PatchOutput struct {
	UpdatedCount int `json:"updated_count"`
}

func patchExecute(ctx context.Context, input PatchInput) (PatchOutput, error) {
	writer := L1WriterFromCtx(ctx)
	reader := L1ReaderFromCtx(ctx)
	sessID := SessionIDFromCtx(ctx)
	agentID := AgentIDFromCtx(ctx)
	if writer == nil || sessID == "" {
		return PatchOutput{}, fmt.Errorf("working_memory not available")
	}
	taskID, err := ensureActiveTask(ctx, writer, reader, sessID, agentID)
	if err != nil {
		return PatchOutput{}, err
	}
	fields := make([]biz.L1FieldInsert, 0, len(input.Fields))
	for _, f := range input.Fields {
		fields = append(fields, biz.L1FieldInsert{
			TaskID:      taskID,
			SessionID:   sessID,
			AgentID:     agentID,
			FieldPath:   f.FieldPath,
			ValueText:   f.Value,
			PinToPrompt: f.Pinned,
			Source:      "agent",
			ChangedBy:   "agent",
		})
	}
	_, err = writer.PatchL1Fields(ctx, fields)
	if err != nil {
		return PatchOutput{}, err
	}
	return PatchOutput{UpdatedCount: len(fields)}, nil
}

// NewPatchTool creates the working_memory.patch tool.
func NewPatchTool() trpctool.Tool {
	return trpcfunction.NewFunctionTool(patchExecute,
		trpcfunction.WithName("working_memory.patch"),
		trpcfunction.WithDescription("Batch update multiple working memory fields at once. More efficient than multiple write calls."),
	)
}

// --- working_memory.delete ---

// DeleteInput is the input for working_memory.delete.
type DeleteInput struct {
	FieldPath string `json:"field_path" jsonschema:"description=The field path to delete,required"`
}

// DeleteOutput is the output for working_memory.delete.
type DeleteOutput struct {
	Deleted bool `json:"deleted"`
}

func deleteExecute(ctx context.Context, input DeleteInput) (DeleteOutput, error) {
	writer := L1WriterFromCtx(ctx)
	reader := L1ReaderFromCtx(ctx)
	sessID := SessionIDFromCtx(ctx)
	agentID := AgentIDFromCtx(ctx)
	if writer == nil || sessID == "" {
		return DeleteOutput{}, fmt.Errorf("working_memory not available")
	}
	taskID, err := findActiveTaskID(ctx, reader, sessID, agentID)
	if err != nil || taskID == "" {
		return DeleteOutput{Deleted: false}, nil
	}
	err = writer.DeleteL1Field(ctx, taskID, input.FieldPath)
	if err != nil {
		return DeleteOutput{Deleted: false}, nil
	}
	return DeleteOutput{Deleted: true}, nil
}

// NewDeleteTool creates the working_memory.delete tool.
func NewDeleteTool() trpctool.Tool {
	return trpcfunction.NewFunctionTool(deleteExecute,
		trpcfunction.WithName("working_memory.delete"),
		trpcfunction.WithDescription("Delete a field from the current working memory task."),
	)
}

// --- ToolSet adapter ---

// ToolSet implements trpctool.ToolSet for the working_memory tool group.
type ToolSet struct{}

// Tools returns all 5 working_memory tools.
func (ToolSet) Tools(_ context.Context) []trpctool.Tool {
	return Tools()
}

// Name returns the toolset name.
func (ToolSet) Name() string {
	return "working_memory"
}

// Close releases resources held by the toolset (none for working_memory).
func (ToolSet) Close() error { return nil }

// Tools returns all 5 working_memory tools as a flat slice.
func Tools() []trpctool.Tool {
	return []trpctool.Tool{
		NewReadTool(),
		NewListTool(),
		NewWriteTool(),
		NewPatchTool(),
		NewDeleteTool(),
	}
}

// Compile-time check: ReadOutput must be JSON-serializable.
var _ json.Marshaler // used implicitly by trpcfunction
