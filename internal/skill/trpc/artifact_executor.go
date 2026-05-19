package trpc

import (
	"context"
	"io/fs"
	"mime"
	"os"
	"path/filepath"
	"strings"

	"trpc.group/trpc-go/trpc-agent-go/codeexecutor"
)

// artifactSavingExecutor wraps a CodeExecutor and persists output files as session artifacts.
type artifactSavingExecutor struct {
	inner codeexecutor.CodeExecutor
}

// WrapWithArtifactSave returns an executor that auto-saves CodeExecutionResult output files.
func WrapWithArtifactSave(inner codeexecutor.CodeExecutor) codeexecutor.CodeExecutor {
	if inner == nil {
		return nil
	}
	return &artifactSavingExecutor{inner: inner}
}

func (w *artifactSavingExecutor) CodeBlockDelimiter() codeexecutor.CodeBlockDelimiter {
	return w.inner.CodeBlockDelimiter()
}

func (w *artifactSavingExecutor) ExecuteCode(ctx context.Context, input codeexecutor.CodeExecutionInput) (codeexecutor.CodeExecutionResult, error) {
	result, err := w.inner.ExecuteCode(ctx, input)
	if err != nil {
		return result, err
	}
	persistOutputFiles(ctx, result.OutputFiles)
	return result, nil
}

func persistOutputFiles(ctx context.Context, files []codeexecutor.File) {
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
		mimeType := strings.TrimSpace(f.MIMEType)
		if mimeType == "" {
			mimeType = mime.TypeByExtension(filepath.Ext(f.Name))
		}
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}
		_, _ = codeexecutor.SaveArtifactHelper(ctx, f.Name, data, mimeType)
	}
}

// collectDockerOutputFiles walks an output directory and returns framework File entries.
func collectDockerOutputFiles(outDir string) []codeexecutor.File {
	if strings.TrimSpace(outDir) == "" {
		return nil
	}
	var files []codeexecutor.File
	_ = filepath.WalkDir(outDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		info, statErr := d.Info()
		if statErr != nil || info.Size() == 0 {
			return nil
		}
		if info.Size() > 10<<20 {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		name := filepath.Base(path)
		mimeType := mime.TypeByExtension(filepath.Ext(name))
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}
		files = append(files, codeexecutor.File{
			Name:      name,
			Content:   string(data),
			MIMEType:  mimeType,
			SizeBytes: info.Size(),
		})
		return nil
	})
	return files
}
