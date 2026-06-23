package working_memory

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

// --- Context keys for dependency injection ---

type l1TaskWriterKey struct{}
type l1FieldWriterKey struct{}
type l1ReaderKey struct{}
type sessionIDKey struct{}
type agentIDKey struct{}
type l1HistoryEnabledKey struct{}
type l1SchemaReaderKey struct{}
type l1DefaultSchemaIDKey struct{}

// WithL1TaskWriter injects L1TaskWriter into context for tool execution.
func WithL1TaskWriter(ctx context.Context, w biz.L1TaskWriter) context.Context {
	return context.WithValue(ctx, l1TaskWriterKey{}, w)
}

// WithL1FieldWriter injects L1FieldWriter into context for tool execution.
func WithL1FieldWriter(ctx context.Context, w biz.L1FieldWriter) context.Context {
	return context.WithValue(ctx, l1FieldWriterKey{}, w)
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

// WithL1HistoryEnabled injects the L1 history archival flag into context for tool execution.
func WithL1HistoryEnabled(ctx context.Context, enabled bool) context.Context {
	return context.WithValue(ctx, l1HistoryEnabledKey{}, enabled)
}

// WithL1SchemaReader injects L1SchemaReader into context for tool execution.
func WithL1SchemaReader(ctx context.Context, r biz.L1SchemaReader) context.Context {
	return context.WithValue(ctx, l1SchemaReaderKey{}, r)
}

// WithL1DefaultSchemaID injects the default L1 schema ID into context for tool execution.
func WithL1DefaultSchemaID(ctx context.Context, schemaID string) context.Context {
	return context.WithValue(ctx, l1DefaultSchemaIDKey{}, schemaID)
}

// L1TaskWriterFromCtx extracts L1TaskWriter from context.
func L1TaskWriterFromCtx(ctx context.Context) biz.L1TaskWriter {
	v, _ := ctx.Value(l1TaskWriterKey{}).(biz.L1TaskWriter)
	return v
}

// L1FieldWriterFromCtx extracts L1FieldWriter from context.
func L1FieldWriterFromCtx(ctx context.Context) biz.L1FieldWriter {
	v, _ := ctx.Value(l1FieldWriterKey{}).(biz.L1FieldWriter)
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

// L1HistoryEnabledFromCtx extracts the L1 history archival flag from context.
func L1HistoryEnabledFromCtx(ctx context.Context) bool {
	v, _ := ctx.Value(l1HistoryEnabledKey{}).(bool)
	return v
}

// L1SchemaReaderFromCtx extracts L1SchemaReader from context.
func L1SchemaReaderFromCtx(ctx context.Context) biz.L1SchemaReader {
	v, _ := ctx.Value(l1SchemaReaderKey{}).(biz.L1SchemaReader)
	return v
}

// L1DefaultSchemaIDFromCtx extracts the default L1 schema ID from context.
func L1DefaultSchemaIDFromCtx(ctx context.Context) string {
	v, _ := ctx.Value(l1DefaultSchemaIDKey{}).(string)
	return v
}

// --- Helpers ---

// sanitizeFieldKind returns the field_kind if valid, otherwise "string".
func sanitizeFieldKind(kind string) string {
	kind = strings.TrimSpace(kind)
	if kind == "" {
		return "string"
	}
	for _, v := range biz.ValidFieldKinds {
		if kind == v {
			return kind
		}
	}
	return "string"
}

// validateFieldAgainstSchema checks whether the given field_path is allowed by the schema.
// The schema_json format is: {"fields": [{"path": "field_name", "kind": "string"}, ...]}.
// If the schema has no "fields" key or it's empty, all fields are allowed (no constraint).
func validateFieldAgainstSchema(schemaJSON []byte, fieldPath, fieldKind string) error {
	if len(schemaJSON) == 0 {
		return nil
	}
	m, err := jsonutil.ParseMap(schemaJSON)
	if err != nil {
		return nil // unparseable schema → soft constraint, allow
	}
	fieldsRaw, ok := m["fields"]
	if !ok {
		return nil // no fields key → no constraint
	}
	fieldsSlice, ok := fieldsRaw.([]any)
	if !ok || len(fieldsSlice) == 0 {
		return nil // empty or wrong type → no constraint
	}
	for _, f := range fieldsSlice {
		fm, ok := f.(map[string]any)
		if !ok {
			continue
		}
		if jsonutil.IfaceStr(fm, "path") == fieldPath {
			return nil // field found in schema → allowed
		}
	}
	return apierror.BadRequest(apierror.DomainWorkingMemory, fmt.Sprintf("field_path %q is not allowed by the schema", fieldPath))
}

// validateFieldWithSchema reads the schema from the reader and validates the field.
// If the schema can't be read or parsed, the write is allowed (soft constraint).
func validateFieldWithSchema(ctx context.Context, reader biz.L1SchemaReader, schemaID, fieldPath, fieldKind string) error {
	if reader == nil || schemaID == "" {
		return nil
	}
	schemaRow, err := reader.GetL1SchemaRow(ctx, schemaID)
	if err != nil {
		// Can't read schema → soft constraint, allow
		return nil
	}
	rowMap, err := jsonutil.ParseMap(schemaRow)
	if err != nil {
		return nil
	}
	schemaJSONStr := jsonutil.IfaceStr(rowMap, "schema_json")
	if schemaJSONStr == "" {
		return nil
	}
	return validateFieldAgainstSchema([]byte(schemaJSONStr), fieldPath, fieldKind)
}

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
func ensureActiveTask(ctx context.Context, writer biz.L1TaskWriter, reader biz.L1AdminReader, sessID, agentID string) (string, error) {
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

// --- read ---

// ReadInput is the input for read.
type ReadInput struct {
	FieldPath string `json:"field_path" jsonschema:"description=The field path to read,required"`
}

// ReadOutput is the output for read.
type ReadOutput struct {
	FieldPath string `json:"field_path"`
	Value     string `json:"value"`
	Found     bool   `json:"found"`
}

func readExecute(ctx context.Context, input ReadInput) (ReadOutput, error) {
	taskWriter := L1TaskWriterFromCtx(ctx)
	fieldWriter := L1FieldWriterFromCtx(ctx)
	reader := L1ReaderFromCtx(ctx)
	sessID := SessionIDFromCtx(ctx)
	agentID := AgentIDFromCtx(ctx)
	if taskWriter == nil || fieldWriter == nil || reader == nil || sessID == "" {
		return ReadOutput{}, apierror.Internal(apierror.DomainWorkingMemory, "working_memory not available")
	}
	taskID, err := findActiveTaskID(ctx, reader, sessID, agentID)
	if err != nil || taskID == "" {
		return ReadOutput{FieldPath: input.FieldPath, Found: false}, nil
	}
	raw, err := reader.GetL1FieldRow(ctx, taskID, input.FieldPath)
	if err != nil {
		return ReadOutput{}, apierror.Internal(apierror.DomainWorkingMemory, "read field: "+err.Error())
	}
	if len(raw) == 0 {
		return ReadOutput{FieldPath: input.FieldPath, Found: false}, nil
	}
	m, _ := jsonutil.ParseMap(raw)
	return ReadOutput{
		FieldPath: input.FieldPath,
		Value:     jsonutil.IfaceStr(m, "value_text"),
		Found:     true,
	}, nil
}

// NewReadTool creates the read tool.
func NewReadTool() trpctool.Tool {
	return trpcfunction.NewFunctionTool(readExecute,
		trpcfunction.WithName("read"),
		trpcfunction.WithDescription("Read a field value from the current working memory task. Use this to retrieve stored context like user preferences, task progress, or intermediate results."),
	)
}

// --- list ---

// ListInput is the input for list.
type ListInput struct{}

// FieldEntry represents a single field in the list output.
type FieldEntry struct {
	FieldPath string `json:"field_path"`
	Value     string `json:"value"`
	Kind      string `json:"kind"`
	Pinned    bool   `json:"pinned"`
}

// ListOutput is the output for list.
type ListOutput struct {
	Fields []FieldEntry `json:"fields"`
}

func listExecute(ctx context.Context, input ListInput) (ListOutput, error) {
	reader := L1ReaderFromCtx(ctx)
	sessID := SessionIDFromCtx(ctx)
	agentID := AgentIDFromCtx(ctx)
	if reader == nil || sessID == "" {
		return ListOutput{}, apierror.Internal(apierror.DomainWorkingMemory, "working_memory not available")
	}
	taskID, err := findActiveTaskID(ctx, reader, sessID, agentID)
	if err != nil || taskID == "" {
		return ListOutput{}, nil
	}
	rows, err := reader.ListL1FieldRows(ctx, taskID, false)
	if err != nil {
		return ListOutput{}, apierror.Internal(apierror.DomainWorkingMemory, "list fields: "+err.Error())
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

// NewListTool creates the list tool.
func NewListTool() trpctool.Tool {
	return trpcfunction.NewFunctionTool(listExecute,
		trpcfunction.WithName("list"),
		trpcfunction.WithDescription("List all fields in the current working memory task. Returns field paths, values, and pin status."),
	)
}

// --- write ---

// WriteInput is the input for write.
type WriteInput struct {
	FieldPath string `json:"field_path" jsonschema:"description=The field path to write to,required"`
	Value     string `json:"value" jsonschema:"description=The value to store,required"`
	FieldKind string `json:"field_kind" jsonschema:"description=The kind of field: string (plain text), number (numeric value), boolean (true/false), json (structured data), reference (pointer to another resource), markdown (rich text), decision (chosen option), artifact (file or output path), progress (completion status), constraint (requirement or limitation)"`
	Pinned    bool   `json:"pinned" jsonschema:"description=Whether to pin this field to the prompt"`
}

// WriteOutput is the output for write.
type WriteOutput struct {
	FieldPath string `json:"field_path"`
	Value     string `json:"value"`
	Revision  int32  `json:"revision"`
}

func writeExecute(ctx context.Context, input WriteInput) (WriteOutput, error) {
	taskWriter := L1TaskWriterFromCtx(ctx)
	fieldWriter := L1FieldWriterFromCtx(ctx)
	reader := L1ReaderFromCtx(ctx)
	sessID := SessionIDFromCtx(ctx)
	agentID := AgentIDFromCtx(ctx)
	if taskWriter == nil || fieldWriter == nil || reader == nil || sessID == "" {
		return WriteOutput{}, apierror.Internal(apierror.DomainWorkingMemory, "working_memory not available")
	}
	// Validate field_kind against known enum values.
	input.FieldKind = sanitizeFieldKind(input.FieldKind)
	taskID, err := ensureActiveTask(ctx, taskWriter, reader, sessID, agentID)
	if err != nil {
		return WriteOutput{}, err
	}
	// Schema validation (soft constraint: skip if schema unavailable)
	if schemaID := L1DefaultSchemaIDFromCtx(ctx); schemaID != "" {
		if schemaReader := L1SchemaReaderFromCtx(ctx); schemaReader != nil {
			if err := validateFieldWithSchema(ctx, schemaReader, schemaID, input.FieldPath, input.FieldKind); err != nil {
				return WriteOutput{}, err
			}
		}
	}
	raw, err := fieldWriter.UpsertL1Field(ctx, biz.L1FieldInsert{
		TaskID:         taskID,
		SessionID:      sessID,
		AgentID:        agentID,
		FieldPath:      input.FieldPath,
		FieldKind:      input.FieldKind,
		ValueText:      input.Value,
		PinToPrompt:    input.Pinned,
		Source:         "agent",
		ChangedBy:      "agent",
		HistoryEnabled: L1HistoryEnabledFromCtx(ctx),
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

// NewWriteTool creates the write tool.
func NewWriteTool() trpctool.Tool {
	return trpcfunction.NewFunctionTool(writeExecute,
		trpcfunction.WithName("write"),
		trpcfunction.WithDescription("Write a field value to the current working memory task. Creates the task if none exists. Pinned fields are included in the prompt for future turns.\n\nRecommended field names by kind:\n- decision: current_approach, chosen_option, architecture_decision\n- artifact: main_file, config_path, output_file\n- progress: current_step, completion_status, next_action\n- constraint: user_requirement, tech_limitation, deadline\n- string/number/boolean/json: any descriptive name (e.g., user_preference, retry_count, is_debug_mode, api_response)"),
	)
}

// --- patch ---

// PatchField represents a single field in a patch batch.
type PatchField struct {
	FieldPath string `json:"field_path" jsonschema:"description=The field path,required"`
	Value     string `json:"value" jsonschema:"description=The value to store,required"`
	FieldKind string `json:"field_kind" jsonschema:"description=The kind of field: string (plain text), number (numeric value), boolean (true/false), json (structured data), reference (pointer to another resource), markdown (rich text), decision (chosen option), artifact (file or output path), progress (completion status), constraint (requirement or limitation)"`
	Pinned    bool   `json:"pinned" jsonschema:"description=Whether to pin this field to the prompt"`
}

// PatchInput is the input for patch.
type PatchInput struct {
	Fields []PatchField `json:"fields" jsonschema:"description=List of fields to upsert,required"`
}

// PatchOutput is the output for patch.
type PatchOutput struct {
	UpdatedCount int `json:"updated_count"`
}

func patchExecute(ctx context.Context, input PatchInput) (PatchOutput, error) {
	taskWriter := L1TaskWriterFromCtx(ctx)
	fieldWriter := L1FieldWriterFromCtx(ctx)
	reader := L1ReaderFromCtx(ctx)
	sessID := SessionIDFromCtx(ctx)
	agentID := AgentIDFromCtx(ctx)
	if taskWriter == nil || fieldWriter == nil || reader == nil || sessID == "" {
		return PatchOutput{}, apierror.Internal(apierror.DomainWorkingMemory, "working_memory not available")
	}
	taskID, err := ensureActiveTask(ctx, taskWriter, reader, sessID, agentID)
	if err != nil {
		return PatchOutput{}, err
	}
	// Schema validation (soft constraint: skip if schema unavailable)
	schemaID := L1DefaultSchemaIDFromCtx(ctx)
	schemaReader := L1SchemaReaderFromCtx(ctx)
	if schemaID != "" && schemaReader != nil {
		for _, f := range input.Fields {
			if err := validateFieldWithSchema(ctx, schemaReader, schemaID, f.FieldPath, f.FieldKind); err != nil {
				return PatchOutput{}, err
			}
		}
	}
	fields := make([]biz.L1FieldInsert, 0, len(input.Fields))
	historyEnabled := L1HistoryEnabledFromCtx(ctx)
	for _, f := range input.Fields {
		f.FieldKind = sanitizeFieldKind(f.FieldKind)
		fields = append(fields, biz.L1FieldInsert{
			TaskID:         taskID,
			SessionID:      sessID,
			AgentID:        agentID,
			FieldPath:      f.FieldPath,
			ValueText:      f.Value,
			FieldKind:      f.FieldKind,
			PinToPrompt:    f.Pinned,
			Source:         "agent",
			ChangedBy:      "agent",
			HistoryEnabled: historyEnabled,
		})
	}
	_, err = fieldWriter.PatchL1Fields(ctx, fields)
	if err != nil {
		return PatchOutput{}, err
	}
	return PatchOutput{UpdatedCount: len(fields)}, nil
}

// NewPatchTool creates the patch tool.
func NewPatchTool() trpctool.Tool {
	return trpcfunction.NewFunctionTool(patchExecute,
		trpcfunction.WithName("patch"),
		trpcfunction.WithDescription("Batch update multiple working memory fields at once. More efficient than multiple write calls."),
	)
}

// --- delete ---

// DeleteInput is the input for delete.
type DeleteInput struct {
	FieldPath string `json:"field_path" jsonschema:"description=The field path to delete,required"`
}

// DeleteOutput is the output for delete.
type DeleteOutput struct {
	Deleted bool `json:"deleted"`
}

func deleteExecute(ctx context.Context, input DeleteInput) (DeleteOutput, error) {
	fieldWriter := L1FieldWriterFromCtx(ctx)
	reader := L1ReaderFromCtx(ctx)
	sessID := SessionIDFromCtx(ctx)
	agentID := AgentIDFromCtx(ctx)
	if fieldWriter == nil || reader == nil || sessID == "" {
		return DeleteOutput{}, apierror.Internal(apierror.DomainWorkingMemory, "working_memory not available")
	}
	taskID, err := findActiveTaskID(ctx, reader, sessID, agentID)
	if err != nil || taskID == "" {
		return DeleteOutput{Deleted: false}, nil
	}
	err = fieldWriter.DeleteL1Field(ctx, taskID, input.FieldPath)
	if err != nil {
		return DeleteOutput{}, apierror.Internal(apierror.DomainWorkingMemory, "delete field: "+err.Error())
	}
	return DeleteOutput{Deleted: true}, nil
}

// NewDeleteTool creates the delete tool.
func NewDeleteTool() trpctool.Tool {
	return trpcfunction.NewFunctionTool(deleteExecute,
		trpcfunction.WithName("delete"),
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
