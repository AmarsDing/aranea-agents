package tool

import "context"

// SanitizeToolInvocationWrite redacts and truncates invocation previews before persistence.
func SanitizeToolInvocationWrite(in *ToolInvocationWrite) {
	if in == nil {
		return
	}
	in.InputPreview = RedactToolPreview(in.InputPreview, toolPreviewMaxLen)
	in.OutputPreview = RedactToolPreview(in.OutputPreview, toolPreviewMaxLen)
	in.ErrorMessage = RedactToolPreview(in.ErrorMessage, 500)
}

// toolParamsJSONMaxLen bounds the redacted params JSON persisted to
// tool_invocation_params (larger than the 2000-char preview budget so the
// 「参数详情」Tab can show near-complete arguments).
const toolParamsJSONMaxLen = 8000

func (u *ToolUsecase) RecordToolInvocation(ctx context.Context, in ToolInvocationWrite) error {
	SanitizeToolInvocationWrite(&in)
	return u.repo.RecordToolInvocation(ctx, in)
}

// RecordToolInvocationParams persists the redacted params row for one
// invocation. Redaction reuses the same chain as invocation previews
// (defense-in-depth: callers already pass redacted JSON).
func (u *ToolUsecase) RecordToolInvocationParams(ctx context.Context, in ToolInvocationParamWrite) error {
	in.ParamsJSON = RedactToolPreview(in.ParamsJSON, toolParamsJSONMaxLen)
	return u.repo.RecordToolInvocationParams(ctx, in)
}
