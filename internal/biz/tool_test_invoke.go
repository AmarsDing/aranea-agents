package biz

import (
	"context"
	"strings"
	"time"

	"aranea-agents/internal/tools/testexec"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

// ToolTestResult is the outcome of TestTool.
type ToolTestResult = testexec.Result

// TestTool executes a single catalog tool call for configuration validation.
func (u *ToolUsecase) TestTool(ctx context.Context, toolID, argumentsJSON string, timeoutSec int) (ToolTestResult, error) {
	toolID = strings.TrimSpace(toolID)
	if toolID == "" {
		return ToolTestResult{}, kerrors.BadRequest("TOOL", "tool id is required")
	}
	tool, err := u.GetTool(ctx, toolID)
	if err != nil {
		return ToolTestResult{}, err
	}
	res, err := testexec.Execute(ctx, testexec.CatalogTool{
		Key:               tool.Key,
		Source:            tool.Source,
		ConfigJSON:        tool.ConfigJSON,
		DefaultConfigJSON: tool.DefaultConfigJSON,
		MetadataJSON:      tool.MetadataJSON,
	}, argumentsJSON, timeoutSec)
	if err != nil {
		return ToolTestResult{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	inputPreview := RedactToolPreview(argumentsJSON, 2000)
	write := ToolInvocationWrite{
		ToolKey:       tool.Key,
		Status:        res.Status,
		DurationMS:    res.DurationMS,
		StartedAt:     now,
		EndedAt:       now,
		InputPreview:  inputPreview,
		OutputPreview: res.ResultPreview,
		ErrorMessage:  res.ErrorMessage,
		Source:        "tool_test",
	}
	_ = u.RecordToolInvocation(context.Background(), write)
	return res, nil
}
