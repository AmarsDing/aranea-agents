package trpc

import (
	"trpc.group/trpc-go/trpc-agent-go/codeexecutor"
	localexec "trpc.group/trpc-go/trpc-agent-go/codeexecutor/local"
)

func NewLocalExecutor(workDir string) codeexecutor.CodeExecutor {
	opts := []localexec.CodeExecutorOption{
		localexec.WithCleanTempFiles(true),
	}
	if workDir != "" {
		opts = append(opts, localexec.WithWorkDir(workDir))
	}
	return localexec.New(opts...)
}
