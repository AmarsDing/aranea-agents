package trpc

import (
	"fmt"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
	trpcfile "trpc.group/trpc-go/trpc-agent-go/tool/file"
	trpchostexec "trpc.group/trpc-go/trpc-agent-go/tool/hostexec"
)

type ToolsetConfig struct {
	Filesystem    bool
	FilesystemDir string
	ShellExec     bool
}

type AssembledToolsets struct {
	ToolSets []trpctool.ToolSet
	Tools    []trpctool.Tool
}

func BuildToolsets(cfg ToolsetConfig) (*AssembledToolsets, error) {
	out := &AssembledToolsets{}

	if cfg.Filesystem {
		opts := []trpcfile.Option{}
		if cfg.FilesystemDir != "" {
			opts = append(opts, trpcfile.WithBaseDir(cfg.FilesystemDir))
		}
		ts, err := trpcfile.NewToolSet(opts...)
		if err != nil {
			return nil, fmt.Errorf("trpc file toolset: %w", err)
		}
		out.ToolSets = append(out.ToolSets, ts)
	}

	if cfg.ShellExec {
		ts, err := trpchostexec.NewToolSet()
		if err != nil {
			return nil, fmt.Errorf("trpc hostexec toolset: %w", err)
		}
		out.ToolSets = append(out.ToolSets, ts)
	}

	return out, nil
}
