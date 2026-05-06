// Package registerstd 在进程启动时向全局 toolapi.Default() 注册全部内置工具实现。
// 请通过空白 import 本包（例如 internal/agent）确保在任何 Invoke/装配发生前完成注册。
package registerstd

import (
	"aranea-agents/internal/tools/edit_file"
	"aranea-agents/internal/tools/exit_loop"
	"aranea-agents/internal/tools/google_search"
	"aranea-agents/internal/tools/list_files"
	"aranea-agents/internal/tools/load_artifacts"
	"aranea-agents/internal/tools/load_memory"
	"aranea-agents/internal/tools/preload_memory"
	"aranea-agents/internal/tools/read_file"
	"aranea-agents/internal/tools/toolapi"
	"aranea-agents/internal/tools/write_file"
)

func init() {
	r := toolapi.Default()
	for _, reg := range []func() toolapi.Tool{
		read_file.New,
		list_files.New,
		write_file.New,
		edit_file.New,
		exit_loop.New,
		google_search.New,
		load_artifacts.New,
		load_memory.New,
		preload_memory.New,
	} {
		r.Register(reg())
	}
}
