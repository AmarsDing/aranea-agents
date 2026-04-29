package registry

import "arenea/backend/internal/capability/backends"

func Builtins() *Registry {
	r := New()
	r.Register(backends.NewDateTimeTool())
	r.Register(backends.NewWebFetchTool())
	r.Register(backends.NewReadFileTool())
	r.Register(backends.NewListFilesTool())
	r.Register(backends.NewWriteFileTool())
	r.Register(backends.NewEditFileTool())
	return r
}
