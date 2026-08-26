package codeexecutor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"trpc.group/trpc-go/trpc-agent-go/codeexecutor"
)

// dockerAdapter adapts DockerExecutor to the framework CodeExecutor interface.
type dockerAdapter struct {
	exec    *DockerExecutor
	timeout time.Duration
}

func newDockerAdapter(cfg DockerConfig, timeout time.Duration) *dockerAdapter {
	return &dockerAdapter{
		exec:    NewDockerExecutor(cfg),
		timeout: timeout,
	}
}

func (a *dockerAdapter) CodeBlockDelimiter() codeexecutor.CodeBlockDelimiter {
	return codeexecutor.CodeBlockDelimiter{Start: "```", End: "```"}
}

func (a *dockerAdapter) ExecuteCode(ctx context.Context, input codeexecutor.CodeExecutionInput) (codeexecutor.CodeExecutionResult, error) {
	var sb strings.Builder
	var outFiles []codeexecutor.File
	for i, block := range input.CodeBlocks {
		res, err := a.exec.Run(ctx, block.Language, block.Code, a.timeout)
		if err != nil {
			return codeexecutor.CodeExecutionResult{}, fmt.Errorf("docker executor block %d: %w", i, err)
		}
		sb.WriteString(formatBlockOutput(res))
		if res.ArtifactDir != "" {
			outFiles = append(outFiles, CollectOutputDirFiles(res.ArtifactDir, DefaultMaxOutputFileBytes)...)
			// Run transfers ArtifactDir ownership to the caller; remove the
			// whole codeexec-* temp tree (the parent of …/out) after collection
			// so artifact dirs don't accumulate.
			_ = os.RemoveAll(filepath.Dir(res.ArtifactDir))
		}
	}
	return codeexecutor.CodeExecutionResult{Output: sb.String(), OutputFiles: outFiles}, nil
}

func formatBlockOutput(res Result) string {
	var sb strings.Builder
	if res.Stdout != "" {
		sb.WriteString(res.Stdout)
	}
	if res.Stderr != "" {
		sb.WriteString(res.Stderr)
	}
	if res.TimedOut {
		sb.WriteString("\n[timeout]")
	}
	if res.OOM {
		sb.WriteString("\n[OOM killed]")
	}
	if res.ExitCode != 0 && !res.TimedOut && !res.OOM {
		sb.WriteString(fmt.Sprintf("\n[exit %d]", res.ExitCode))
	}
	return sb.String()
}
