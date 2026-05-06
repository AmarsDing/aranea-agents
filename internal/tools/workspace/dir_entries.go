package workspace

import "os"

// DirEntriesAsItems 把 os.DirEntry 列表转为可被 JSON/map 输出的简要信息（仅供工作区工具重用）。
func DirEntriesAsItems(entries []os.DirEntry) []map[string]any {
	items := make([]map[string]any, 0, len(entries))
	for _, entry := range entries {
		item := map[string]any{"name": entry.Name(), "isDir": entry.IsDir()}
		if info, statErr := entry.Info(); statErr == nil {
			item["size"] = info.Size()
			item["modTime"] = info.ModTime().Format("2006-01-02T15:04:05Z07:00")
		}
		items = append(items, item)
	}
	return items
}
