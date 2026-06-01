package trpc

import (
	"context"
	"mime"
	"path/filepath"
	"strings"

	"aranea-agents/internal/biz/artifact"
	"aranea-agents/pkg/loggateway"

	"trpc.group/trpc-go/trpc-agent-go/codeexecutor"
)

// artifactSavingExecutor wraps a CodeExecutor and persists output files as session artifacts.
type artifactSavingExecutor struct {
	inner codeexecutor.CodeExecutor
	lg    loggateway.Logger
}

// WrapWithArtifactSave returns an executor that auto-saves CodeExecutionResult output files.
func WrapWithArtifactSave(inner codeexecutor.CodeExecutor, lg loggateway.Logger) codeexecutor.CodeExecutor {
	if inner == nil {
		return nil
	}
	return &artifactSavingExecutor{inner: inner, lg: lg}
}

func (w *artifactSavingExecutor) CodeBlockDelimiter() codeexecutor.CodeBlockDelimiter {
	return w.inner.CodeBlockDelimiter()
}

func (w *artifactSavingExecutor) ExecuteCode(ctx context.Context, input codeexecutor.CodeExecutionInput) (codeexecutor.CodeExecutionResult, error) {
	result, err := w.inner.ExecuteCode(ctx, input)
	if err != nil {
		return result, err
	}
	persistOutputFiles(ctx, result.OutputFiles, w.lg)
	return result, nil
}

func persistOutputFiles(ctx context.Context, files []codeexecutor.File, lg loggateway.Logger) {
	if len(files) == 0 {
		return
	}
	for _, f := range files {
		if strings.TrimSpace(f.Name) == "" {
			continue
		}
		data := []byte(f.Content)
		if len(data) == 0 {
			continue
		}
		if len(data) > artifact.MaxUploadBytes {
			lg.Warn("代码执行产出物超过 10 MB，已跳过",
				loggateway.StepID("codeexec.artifact_save"),
				loggateway.Str("filename", f.Name),
				loggateway.Int("size", len(data)))
			continue
		}
		mimeType := strings.TrimSpace(f.MIMEType)
		if mimeType == "" {
			mimeType = mime.TypeByExtension(filepath.Ext(f.Name))
		}
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}
		if _, err := codeexecutor.SaveArtifactHelper(ctx, f.Name, data, mimeType); err != nil {
			lg.Warn("代码执行产出物保存失败",
				loggateway.StepID("codeexec.artifact_save"),
				loggateway.Str("filename", f.Name),
				loggateway.Err(err))
		}
	}
}
