// Package memberfs provides the 3 read-only workspace tools assembled for
// department lead agents (M71). Each tool is a thin shell: input validation →
// biz usecase delegation (with the caller's agent ID bound at assembly time) →
// result mapping.
package memberfs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// Accessor is the biz-layer port backing the memberfs tools.
// *biz.ResourceAccessUsecase satisfies it.
type Accessor interface {
	ListMemberFiles(ctx context.Context, callerAgentID, targetAgentKey, subdir string, depth int) ([]biz.FileEntry, error)
	ReadMemberFile(ctx context.Context, callerAgentID, targetAgentKey, rel string, maxBytes int64) (string, bool, error)
	SearchMemberFiles(ctx context.Context, callerAgentID, targetAgentKey, pattern string, limit int) ([]string, error)
}

// RegisterAll creates the 3 memberfs tools bound to the caller's identity.
func RegisterAll(acc Accessor, callerAgentID string, lg loggateway.Logger) []trpctool.Tool {
	if acc == nil || strings.TrimSpace(callerAgentID) == "" {
		return nil
	}
	return []trpctool.Tool{
		&listMemberFilesTool{acc: acc, callerID: callerAgentID, lg: lg},
		&readMemberFileTool{acc: acc, callerID: callerAgentID, lg: lg},
		&searchMemberFilesTool{acc: acc, callerID: callerAgentID, lg: lg},
	}
}

// ---------------------------------------------------------------------------
// list_member_files
// ---------------------------------------------------------------------------

type listMemberFilesTool struct {
	acc      Accessor
	callerID string
	lg       loggateway.Logger
}

type listMemberFilesInput struct {
	AgentKey string `json:"agent_key" jsonschema:"description=目标员工的 agent_key（本部门成员或借调到本部门团队的成员）,required"`
	Subdir   string `json:"subdir" jsonschema:"description=起始子目录（相对路径，默认工作目录根）"`
	Depth    int    `json:"depth" jsonschema:"description=递归深度，默认 2，上限 4"`
}

type listMemberFilesOutput struct {
	Entries []biz.FileEntry `json:"entries"`
	Count   int             `json:"count"`
}

func (t *listMemberFilesTool) Declaration() *trpctool.Declaration {
	return &trpctool.Declaration{
		Name: "list_member_files",
		Description: "列出本部门成员（或借调到本部门团队的成员）工作目录的文件树。" +
			"用于了解员工产出物的分布；只读访问，全程审计。",
		InputSchema: &trpctool.Schema{
			Type:     "object",
			Required: []string{"agent_key"},
			Properties: map[string]*trpctool.Schema{
				"agent_key": {Type: "string", Description: "目标员工的 agent_key。"},
				"subdir":    {Type: "string", Description: "起始子目录（相对路径，默认工作目录根）。"},
				"depth":     {Type: "integer", Description: fmt.Sprintf("递归深度，默认 %d，上限 %d。", biz.DefaultMemberDirDepth, biz.MaxMemberDirDepth)},
			},
		},
		OutputSchema: &trpctool.Schema{
			Type:        "object",
			Description: "目录树条目（相对路径，按路径排序）。",
			Required:    []string{"entries", "count"},
			Properties: map[string]*trpctool.Schema{
				"entries": {Type: "array", Description: "文件/目录条目列表（path/is_dir/size）。"},
				"count":   {Type: "integer", Description: "条目总数。"},
			},
		},
	}
}

func (t *listMemberFilesTool) Call(ctx context.Context, jsonArgs []byte) (any, error) {
	var in listMemberFilesInput
	if err := json.Unmarshal(jsonArgs, &in); err != nil {
		return nil, fmt.Errorf("参数解析失败: %w", err)
	}
	entries, err := t.acc.ListMemberFiles(ctx, t.callerID, strings.TrimSpace(in.AgentKey), in.Subdir, in.Depth)
	if err != nil {
		t.logWarn("list_member_files", in.AgentKey, err)
		return nil, err
	}
	return listMemberFilesOutput{Entries: entries, Count: len(entries)}, nil
}

func (t *listMemberFilesTool) logWarn(step, target string, err error) {
	if t.lg != nil {
		t.lg.Warn("memberfs 工具调用失败",
			loggateway.StepID("tool."+step),
			loggateway.Str("target_agent_key", target),
			loggateway.Err(err))
	}
}

// ---------------------------------------------------------------------------
// read_member_file
// ---------------------------------------------------------------------------

type readMemberFileTool struct {
	acc      Accessor
	callerID string
	lg       loggateway.Logger
}

type readMemberFileInput struct {
	AgentKey string `json:"agent_key" jsonschema:"description=目标员工的 agent_key,required"`
	Path     string `json:"path" jsonschema:"description=文件相对路径（相对工作目录根）,required"`
	MaxBytes int64  `json:"max_bytes" jsonschema:"description=最大读取字节数，默认/上限 204800（200KB）"`
}

type readMemberFileOutput struct {
	Content   string `json:"content"`
	Truncated bool   `json:"truncated"`
	Path      string `json:"path"`
}

func (t *readMemberFileTool) Declaration() *trpctool.Declaration {
	return &trpctool.Declaration{
		Name: "read_member_file",
		Description: "读取本部门成员（或借调到本部门团队的成员）工作目录中的文本文件内容。" +
			"二进制文件与非 UTF-8 文件会被拒绝；超过 200KB 截断并附 truncated 标记。只读访问，全程审计。",
		InputSchema: &trpctool.Schema{
			Type:     "object",
			Required: []string{"agent_key", "path"},
			Properties: map[string]*trpctool.Schema{
				"agent_key": {Type: "string", Description: "目标员工的 agent_key。"},
				"path":      {Type: "string", Description: "文件相对路径（相对工作目录根）。"},
				"max_bytes": {Type: "integer", Description: "最大读取字节数，默认/上限 204800（200KB）。"},
			},
		},
		OutputSchema: &trpctool.Schema{
			Type:        "object",
			Description: "文件文本内容（可能截断）。",
			Required:    []string{"content", "truncated", "path"},
			Properties: map[string]*trpctool.Schema{
				"content":   {Type: "string", Description: "文件文本内容。"},
				"truncated": {Type: "boolean", Description: "内容是否被截断到 max_bytes。"},
				"path":      {Type: "string", Description: "实际读取的相对路径。"},
			},
		},
	}
}

func (t *readMemberFileTool) Call(ctx context.Context, jsonArgs []byte) (any, error) {
	var in readMemberFileInput
	if err := json.Unmarshal(jsonArgs, &in); err != nil {
		return nil, fmt.Errorf("参数解析失败: %w", err)
	}
	rel := strings.TrimSpace(in.Path)
	if rel == "" {
		return nil, errors.New("path 为必填项")
	}
	content, truncated, err := t.acc.ReadMemberFile(ctx, t.callerID, strings.TrimSpace(in.AgentKey), rel, in.MaxBytes)
	if err != nil {
		if t.lg != nil {
			t.lg.Warn("memberfs 工具调用失败",
				loggateway.StepID("tool.read_member_file"),
				loggateway.Str("target_agent_key", in.AgentKey),
				loggateway.Err(err))
		}
		return nil, err
	}
	return readMemberFileOutput{Content: content, Truncated: truncated, Path: rel}, nil
}

// ---------------------------------------------------------------------------
// search_member_files
// ---------------------------------------------------------------------------

type searchMemberFilesTool struct {
	acc      Accessor
	callerID string
	lg       loggateway.Logger
}

type searchMemberFilesInput struct {
	AgentKey string `json:"agent_key" jsonschema:"description=目标员工的 agent_key,required"`
	Pattern  string `json:"pattern" jsonschema:"description=glob 匹配模式（匹配文件名或相对路径，如 *.md 或 reports/*）,required"`
}

type searchMemberFilesOutput struct {
	Matches []string `json:"matches"`
	Count   int      `json:"count"`
}

func (t *searchMemberFilesTool) Declaration() *trpctool.Declaration {
	return &trpctool.Declaration{
		Name: "search_member_files",
		Description: "按 glob 模式搜索本部门成员（或借调到本部门团队的成员）工作目录中的文件名。" +
			"返回匹配的相对路径清单（上限 200 条）。只读访问，全程审计。",
		InputSchema: &trpctool.Schema{
			Type:     "object",
			Required: []string{"agent_key", "pattern"},
			Properties: map[string]*trpctool.Schema{
				"agent_key": {Type: "string", Description: "目标员工的 agent_key。"},
				"pattern":   {Type: "string", Description: "glob 匹配模式（如 *.md、reports/*、summary_*.json）。"},
			},
		},
		OutputSchema: &trpctool.Schema{
			Type:        "object",
			Description: "匹配的相对路径清单。",
			Required:    []string{"matches", "count"},
			Properties: map[string]*trpctool.Schema{
				"matches": {Type: "array", Description: "匹配文件的相对路径列表。"},
				"count":   {Type: "integer", Description: "匹配总数。"},
			},
		},
	}
}

func (t *searchMemberFilesTool) Call(ctx context.Context, jsonArgs []byte) (any, error) {
	var in searchMemberFilesInput
	if err := json.Unmarshal(jsonArgs, &in); err != nil {
		return nil, fmt.Errorf("参数解析失败: %w", err)
	}
	matches, err := t.acc.SearchMemberFiles(ctx, t.callerID, strings.TrimSpace(in.AgentKey), in.Pattern, 0)
	if err != nil {
		if t.lg != nil {
			t.lg.Warn("memberfs 工具调用失败",
				loggateway.StepID("tool.search_member_files"),
				loggateway.Str("target_agent_key", in.AgentKey),
				loggateway.Err(err))
		}
		return nil, err
	}
	return searchMemberFilesOutput{Matches: matches, Count: len(matches)}, nil
}

// --- interface guards ---

var (
	_ trpctool.Tool         = (*listMemberFilesTool)(nil)
	_ trpctool.CallableTool = (*listMemberFilesTool)(nil)
	_ trpctool.Tool         = (*readMemberFileTool)(nil)
	_ trpctool.CallableTool = (*readMemberFileTool)(nil)
	_ trpctool.Tool         = (*searchMemberFilesTool)(nil)
	_ trpctool.CallableTool = (*searchMemberFilesTool)(nil)
)
