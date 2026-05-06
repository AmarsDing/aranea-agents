package toolapi

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"google.golang.org/adk/tool"
)

// Registry 维护已注册工具的查找顺序（按 Register 先后顺序）。
type Registry struct {
	mu     sync.RWMutex
	order  []string
	byName map[string]Tool
}

var (
	globalOnce sync.Once
	globalReg  *Registry
)

// Default 进程内全局 Registry（须在 import 一侧通过 registerstd/init 装入内置工具）。
func Default() *Registry {
	globalOnce.Do(func() {
		globalReg = NewRegistry()
	})
	return globalReg
}

// NewRegistry 创建空的工具表（测试或自建隔离表时使用）。
func NewRegistry() *Registry {
	return &Registry{byName: make(map[string]Tool)}
}

// Register 注册工具；同名重复或空名 panic（启动期硬错误）。
func (r *Registry) Register(t Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	name := strings.TrimSpace(t.Meta().Name)
	if name == "" {
		panic("toolapi.Register: Meta().Name empty")
	}
	if _, dup := r.byName[name]; dup {
		panic("toolapi.Register: duplicate tool " + name)
	}
	r.byName[name] = t
	r.order = append(r.order, name)
}

// Tool 按键名（英文函数名）查找。
func (r *Registry) Tool(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.byName[name]
	return t, ok
}

// OrderedNames 返回注册顺序上的全部名称。
func (r *Registry) OrderedNames() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, len(r.order))
	copy(out, r.order)
	return out
}

// Invoke 进程内统一调用路径：仅用 SupportsLocalInvoke 为真的工具。
func (r *Registry) Invoke(ctx context.Context, req InvokeRequest) InvokeResponse {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return ErrorResponse("工具名为空")
	}
	tk, ok := r.Tool(name)
	if !ok {
		return ErrorResponse(fmt.Sprintf("未知工具 %q", name))
	}
	if req.Arguments == nil {
		req.Arguments = map[string]any{}
	}
	if !tk.SupportsLocalInvoke() {
		return ErrorResponse(fmt.Sprintf("工具 %q 仅在 ADK Runner 内可调", name))
	}
	res, err := tk.InvokeLocal(ctx, req.Arguments)
	if err != nil {
		return ErrorResponse(err.Error())
	}
	if res == nil {
		res = map[string]any{}
	}
	return SuccessResponse(res)
}

// InvokeJSON 解析函数参数 JSON（OpenAI/adk payload）后 Invoke。
func (r *Registry) InvokeJSON(ctx context.Context, name, argsJSON string) InvokeResponse {
	var args map[string]any
	if strings.TrimSpace(argsJSON) != "" {
		if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
			return ErrorResponse(fmt.Sprintf("无效的 JSON 参数: %v", err))
		}
	}
	return r.Invoke(ctx, InvokeRequest{Name: name, Arguments: args})
}

// WorkspaceOpenAISpecs 仅导出 WorkspaceToolNames 子集且有 OpenAISpec 的条目（管理端文件工具）。
func (r *Registry) WorkspaceOpenAISpecs(enabled map[string]bool) []map[string]any {
	var out []map[string]any
	for _, key := range WorkspaceToolNames {
		if enabled != nil && !enabled[key] {
			continue
		}
		tk, ok := r.Tool(key)
		if !ok {
			continue
		}
		spec := tk.OpenAIFunction()
		if spec == nil {
			continue
		}
		out = append(out, spec)
	}
	return out
}

// WorkspaceADKTools 仅装配工作区内置工具对应的 ADK tool.Tool。
func (r *Registry) WorkspaceADKTools(enabled map[string]bool) ([]tool.Tool, error) {
	var out []tool.Tool
	for _, key := range WorkspaceToolNames {
		if enabled != nil && !enabled[key] {
			continue
		}
		tk, ok := r.Tool(key)
		if !ok {
			continue
		}
		adkTool, err := tk.ADKTool()
		if err != nil {
			return nil, err
		}
		if adkTool != nil {
			out = append(out, adkTool)
		}
	}
	return out, nil
}

// AllADKTools 按注册顺序收集全部非空的 ADK 工具（跳过 OpenAI-only 或未实现）。
func (r *Registry) AllADKTools(enabled map[string]bool) ([]tool.Tool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []tool.Tool
	for _, key := range r.order {
		if enabled != nil && !enabled[key] {
			continue
		}
		tk := r.byName[key]
		adkTool, err := tk.ADKTool()
		if err != nil {
			return nil, err
		}
		if adkTool != nil {
			out = append(out, adkTool)
		}
	}
	return out, nil
}
