package agent

import (
	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcclaudecode "trpc.group/trpc-go/trpc-agent-go/agent/claudecode"
)

type ClaudeCodeAgentConfig struct {
	Name      string
	Bin       string
	WorkDir   string
	ExtraArgs []string
	Env       []string
}

func BuildClaudeCodeAgent(cfg ClaudeCodeAgentConfig) (trpcagent.Agent, error) {
	var opts []trpcclaudecode.Option
	if cfg.Name != "" {
		opts = append(opts, trpcclaudecode.WithName(cfg.Name))
	}
	if cfg.Bin != "" {
		opts = append(opts, trpcclaudecode.WithBin(cfg.Bin))
	}
	if cfg.WorkDir != "" {
		opts = append(opts, trpcclaudecode.WithWorkDir(cfg.WorkDir))
	}
	if len(cfg.ExtraArgs) > 0 {
		opts = append(opts, trpcclaudecode.WithExtraArgs(cfg.ExtraArgs...))
	}
	if len(cfg.Env) > 0 {
		opts = append(opts, trpcclaudecode.WithEnv(cfg.Env...))
	}
	return trpcclaudecode.New(opts...)
}
