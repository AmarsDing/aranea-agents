package registry

import (
	"aranea-agents/internal/tools/shell_exec"

	"google.golang.org/adk/tool"
)

// ADKToolsFromEnabled builds []tool.Tool for llmagent from an effective-tool key set.
// Keys should already reflect biz policy (only keys the agent may use). Use [ApplyEffectiveAliases] first.
func ADKToolsFromEnabled(enabled map[string]bool) ([]tool.Tool, error) {
	if len(enabled) == 0 {
		return nil, nil
	}
	var out []tool.Tool
	fs, err := WorkspaceADKTools(enabled)
	if err != nil {
		return nil, err
	}
	out = append(out, fs...)

	for _, name := range ADKBuiltinOrder {
		if !enabled[name] {
			continue
		}
		if err := AppendADKBuiltin(name, nil, &out); err != nil {
			return nil, err
		}
	}

	// Mount at most one host shell tool. Policy aliases set both shell + shell_exec in the enabled
	// map; exposing two names makes models alternate calls (shell / shell_exec) in a tight loop.
	if enabled[ShellExec] || enabled[ShellAlias] {
		st, err := shell_exec.New()
		if err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, nil
}
