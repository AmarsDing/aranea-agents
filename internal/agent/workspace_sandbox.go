package agent

import "aranea-agents/internal/tools/workspace"

// WorkspaceRoot 返回工作区工具沙箱的根目录绝对路径。
func WorkspaceRoot() (string, error) {
	return workspace.Root()
}

// ResolveWorkspacePath 将相对于工作区根的路径解析为受限绝对路径与相对片段。
func ResolveWorkspacePath(rawPath string) (absPath string, relPath string, err error) {
	return workspace.ResolvePath(rawPath)
}
