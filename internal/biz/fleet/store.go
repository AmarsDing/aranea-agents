package fleet

import (
	"context"
	"encoding/json"
	"sort"
)

// Store 是运行台的文档存储窄口：按 (kind, id) 存取 JSON 文档。
// 运行台状态量小（几十到几百条）、结构随功能迭代，与 Orca 用 JSON 文件
// 存状态同理；data 层落一张 fleet_records 表。
// Stability:evolving
type Store interface {
	Put(ctx context.Context, kind, id string, doc []byte) error
	Get(ctx context.Context, kind, id string) ([]byte, bool, error)
	List(ctx context.Context, kind string) ([][]byte, error)
	Delete(ctx context.Context, kind, id string) error
}

// ProjectSource 是项目登记（coding_projects）的窄口，由装配层适配 agentbridge.ProjectRepo。
// Stability:evolving
type ProjectSource interface {
	ListProjects(ctx context.Context) ([]Project, error)
	AddProject(ctx context.Context, name, path string) (Project, error)
}

func putDoc(ctx context.Context, s Store, kind, id string, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return s.Put(ctx, kind, id, raw)
}

func getDoc[T any](ctx context.Context, s Store, kind, id string) (T, bool, error) {
	var out T
	raw, ok, err := s.Get(ctx, kind, id)
	if err != nil || !ok {
		return out, ok, err
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return out, false, err
	}
	return out, true, nil
}

func listDocs[T any](ctx context.Context, s Store, kind string) ([]T, error) {
	raws, err := s.List(ctx, kind)
	if err != nil {
		return nil, err
	}
	out := make([]T, 0, len(raws))
	for _, raw := range raws {
		var v T
		if json.Unmarshal(raw, &v) == nil {
			out = append(out, v)
		}
	}
	return out, nil
}

// sortByCreated 按创建时间排序（新的在后）；RFC3339 字符串可直接比较。
func sortByCreated[T any](items []T, created func(T) string) {
	sort.SliceStable(items, func(i, j int) bool { return created(items[i]) < created(items[j]) })
}
