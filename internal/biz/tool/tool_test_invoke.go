package tool

import (
	"context"
	"strings"
	"time"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

type ToolTestResult struct {
	Status        string
	ResultPreview string
	ErrorMessage  string
	DurationMS    int
}

// ToolTestInput is the minimal tool row needed for a test invocation.
type ToolTestInput struct {
	Key               string
	Source            string
	ConfigJSON        string
	DefaultConfigJSON string
	MetadataJSON      string
}

// ToolTester abstracts the execution of a single catalog tool test call.
type ToolTester interface {
	Execute(ctx context.Context, tool ToolTestInput, argumentsJSON string, timeoutSec int, platform *WebResearchPlatformFields) (ToolTestResult, error)
}

// TestTool executes a single catalog tool call for configuration validation.
func (u *ToolUsecase) TestTool(ctx context.Context, toolID, argumentsJSON string, timeoutSec int) (ToolTestResult, error) {
	toolID = strings.TrimSpace(toolID)
	if toolID == "" {
		return ToolTestResult{}, apierror.BadRequest("TOOL", "tool id is required")
	}
	tool, err := u.GetTool(ctx, toolID)
	if err != nil {
		return ToolTestResult{}, err
	}
	if u.tester == nil {
		return ToolTestResult{}, apierror.Internal("TOOL", "tool tester not configured")
	}
	var pf *WebResearchPlatformFields
	if platform := LoadWebResearchPlatform(ctx, u.sys); platform != nil {
		pf = webResearchPlatformFields(*platform)
	}
	res, err := u.tester.Execute(ctx, ToolTestInput{
		Key:               tool.Key,
		Source:            tool.Source,
		ConfigJSON:        tool.ConfigJSON,
		DefaultConfigJSON: tool.DefaultConfigJSON,
		MetadataJSON:      tool.MetadataJSON,
	}, argumentsJSON, timeoutSec, pf)
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
	recordCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := u.RecordToolInvocation(recordCtx, write); err != nil {
		u.lg.Warn("tools.test.record_invocation_failed",
			loggateway.StepID("tool.test_record_fail"),
			loggateway.Str("tool_key", write.ToolKey),
			loggateway.Err(err))
	}
	return res, nil
}
