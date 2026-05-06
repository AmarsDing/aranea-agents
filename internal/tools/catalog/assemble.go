package catalog

import (
	"context"
	"fmt"
	iofs "io/fs"

	"aranea-agents/internal/tools/toolapi"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/agenttool"
	"google.golang.org/adk/tool/exampletool"
	"google.golang.org/adk/tool/mcptoolset"
	"google.golang.org/adk/tool/skilltoolset"
	"google.golang.org/adk/tool/skilltoolset/skill"
)

func appendADKRegistered(r *toolapi.Registry, name string, slice *[]tool.Tool) error {
	tk, ok := r.Tool(name)
	if !ok {
		return fmt.Errorf("catalog: 工具 %q 未在 global registry 注册（请 import internal/tools/registerstd）", name)
	}
	adk, err := tk.ADKTool()
	if err != nil {
		return fmt.Errorf("catalog: %q ADK 构造失败: %w", name, err)
	}
	if adk == nil {
		return fmt.Errorf("catalog: %q 无 ADKTool 绑定", name)
	}
	*slice = append(*slice, adk)
	return nil
}

// SubAgent 将子 Agent 作为 tool 暴露（ADK agenttool）。
type SubAgent struct {
	Agent  agent.Agent
	Config *agenttool.Config
}

// Options 选择要装配到 llmagent 的固定工具与动态 Toolset。
type Options struct {
	ExitLoop      bool
	GoogleSearch  bool
	LoadArtifacts bool
	LoadMemory    bool
	PreloadMemory bool
	Filesystem    bool

	// FilesystemKeys 非 nil 时仅装配其中值为 true 的本仓工作区文件工具（与旧行为一致）。
	FilesystemKeys map[string]bool

	Examples *exampletool.ExampleToolConfig
	SkillsFS iofs.FS
	MCP      *mcptoolset.Config

	SubAgents []SubAgent
}

type Assembled struct {
	Tools    []tool.Tool
	Toolsets []tool.Toolset
}

// Build 从全局 registry（须已 import registerstd）与可选 skill/mcp/agent 组合出工具列表。
func (o Options) Build(ctx context.Context) (*Assembled, error) {
	out := &Assembled{}
	reg := toolapi.Default()

	if o.ExitLoop {
		if err := appendADKRegistered(reg, NameExitLoop, &out.Tools); err != nil {
			return nil, err
		}
	}
	if o.GoogleSearch {
		if err := appendADKRegistered(reg, NameGoogleSearch, &out.Tools); err != nil {
			return nil, err
		}
	}
	if o.LoadArtifacts {
		if err := appendADKRegistered(reg, NameLoadArtifacts, &out.Tools); err != nil {
			return nil, err
		}
	}
	if o.LoadMemory {
		if err := appendADKRegistered(reg, NameLoadMemory, &out.Tools); err != nil {
			return nil, err
		}
	}
	if o.PreloadMemory {
		if err := appendADKRegistered(reg, NamePreloadMemory, &out.Tools); err != nil {
			return nil, err
		}
	}
	if o.Filesystem {
		fsTools, err := reg.WorkspaceADKTools(o.FilesystemKeys)
		if err != nil {
			return nil, fmt.Errorf("工作区文件工具: %w", err)
		}
		out.Tools = append(out.Tools, fsTools...)
	}
	if o.Examples != nil {
		t, err := ExampleFewShot(*o.Examples)
		if err != nil {
			return nil, err
		}
		out.Tools = append(out.Tools, t)
	}
	for _, sub := range o.SubAgents {
		if sub.Agent == nil {
			continue
		}
		out.Tools = append(out.Tools, WrapAgent(sub.Agent, sub.Config))
	}
	if o.SkillsFS != nil {
		ts, err := skilltoolset.New(ctx, skilltoolset.Config{Source: skill.NewFileSystemSource(o.SkillsFS)})
		if err != nil {
			return nil, err
		}
		out.Toolsets = append(out.Toolsets, ts)
	}
	if o.MCP != nil {
		ts, err := mcptoolset.New(*o.MCP)
		if err != nil {
			return nil, err
		}
		out.Toolsets = append(out.Toolsets, ts)
	}
	return out, nil
}
