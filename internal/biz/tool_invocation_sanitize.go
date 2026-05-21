package biz

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

func (u *ToolUsecase) RecordToolInvocation(ctx context.Context, in ToolInvocationWrite) error {
	SanitizeToolInvocationWrite(&in)
	return u.repo.RecordToolInvocation(ctx, in)
}
