package trpc

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	localexec "aranea-agents/internal/agent/codeexecutor"

	"trpc.group/trpc-go/trpc-agent-go/codeexecutor"
	trpclocal "trpc.group/trpc-go/trpc-agent-go/codeexecutor/local"
)

func NewLocalExecutor(workDir string) codeexecutor.CodeExecutor {
	opts := []trpclocal.CodeExecutorOption{
		trpclocal.WithCleanTempFiles(true),
	}
	if workDir != "" {
		opts = append(opts, trpclocal.WithWorkDir(workDir))
	}
	return trpclocal.New(opts...)
}

// NewExecutor returns a codeexecutor.CodeExecutor based on the CODE_EXECUTOR_BACKEND
// environment variable (or the provided backend string). Recognised values:
//   - "docker" → DockerExecutor (EP-BIZ-02)
//   - anything else → local subprocess executor (default)
func NewExecutor(backend, workDir string) codeexecutor.CodeExecutor {
	if strings.TrimSpace(backend) == "" {
		backend = strings.TrimSpace(os.Getenv("CODE_EXECUTOR_BACKEND"))
	}
	var exec codeexecutor.CodeExecutor
	if strings.EqualFold(backend, "docker") {
		exec = newDockerExecutorAdapter()
	} else {
		exec = NewLocalExecutor(workDir)
	}
	return WrapWithArtifactSave(exec)
}

// dockerExecutorAdapter adapts internal/agent/codeexecutor.DockerExecutor to the
// framework's codeexecutor.CodeExecutor interface.
type dockerExecutorAdapter struct {
	exec    *localexec.DockerExecutor
	timeout time.Duration
}

func newDockerExecutorAdapter() *dockerExecutorAdapter {
	cfg := localexec.DefaultDockerConfig()
	image := strings.TrimSpace(os.Getenv("CODE_EXECUTOR_DOCKER_IMAGE"))
	if image != "" {
		cfg.Image = image
	}
	timeout := 60 * time.Second
	if raw := strings.TrimSpace(os.Getenv("CODE_EXECUTOR_TIMEOUT")); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil && d > 0 {
			timeout = d
		}
	}
	return &dockerExecutorAdapter{exec: localexec.NewDockerExecutor(cfg), timeout: timeout}
}

// CodeBlockDelimiter returns standard markdown triple-backtick delimiters.
func (a *dockerExecutorAdapter) CodeBlockDelimiter() codeexecutor.CodeBlockDelimiter {
	return codeexecutor.CodeBlockDelimiter{Start: "```", End: "```"}
}

// ExecuteCode runs each code block sequentially and concatenates the output.
func (a *dockerExecutorAdapter) ExecuteCode(ctx context.Context, input codeexecutor.CodeExecutionInput) (codeexecutor.CodeExecutionResult, error) {
	var sb strings.Builder
	var outFiles []codeexecutor.File
	for i, block := range input.CodeBlocks {
		res, err := a.exec.Run(ctx, block.Language, block.Code, a.timeout)
		if err != nil {
			return codeexecutor.CodeExecutionResult{}, fmt.Errorf("docker executor block %d: %w", i, err)
		}
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
		if res.ArtifactDir != "" {
			outFiles = append(outFiles, collectDockerOutputFiles(res.ArtifactDir)...)
		}
	}
	return codeexecutor.CodeExecutionResult{Output: sb.String(), OutputFiles: outFiles}, nil
}
