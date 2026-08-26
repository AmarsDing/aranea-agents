package decision

import (
	"context"
	"testing"
)

// TestListFilter_NormalizePage pins the pagination clamp contract (设计 §4.1
// page_size ≤ 100；page<1 → 1；0/越界 page_size → 默认/截断）。
func TestListFilter_NormalizePage(t *testing.T) {
	cases := []struct {
		name           string
		page, pageSize int
		wantLimit      int
		wantOffset     int
	}{
		{"zero values get defaults", 0, 0, 20, 0},
		{"negative page clamps to 1", -3, 10, 10, 0},
		{"page_size over cap truncates", 1, 500, 100, 0},
		{"normal page computes offset", 3, 20, 20, 40},
		{"page_size 1 allowed", 2, 1, 1, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := ListFilter{Page: tc.page, PageSize: tc.pageSize}
			limit, offset := f.NormalizePage()
			if limit != tc.wantLimit || offset != tc.wantOffset {
				t.Errorf("NormalizePage = (%d,%d), want (%d,%d)", limit, offset, tc.wantLimit, tc.wantOffset)
			}
		})
	}
}

// TestQueryUsecase_NilRepo: CLI/无库形态下查询面返回空结果而非 panic。
func TestQueryUsecase_NilRepo(t *testing.T) {
	u := NewQueryUsecase(nil)
	items, total, err := u.List(context.Background(), ListFilter{Page: 1, PageSize: 10})
	if err != nil || items != nil || total != 0 {
		t.Errorf("List on nil repo = (%v,%d,%v), want empty", items, total, err)
	}
	rec, err := u.Get(context.Background(), "dk-1")
	if err != nil || rec != nil {
		t.Errorf("Get on nil repo = (%v,%v), want nil/nil", rec, err)
	}
}

// TestQueryUsecase_GetEmptyKey: 空 decision_key 短路为 nil（不打存储）。
func TestQueryUsecase_GetEmptyKey(t *testing.T) {
	u := NewQueryUsecase(&fakeQueryRepo{})
	rec, err := u.Get(context.Background(), "  ")
	if err != nil || rec != nil {
		t.Errorf("Get blank key = (%v,%v), want nil/nil", rec, err)
	}
}

// fakeQueryRepo 是 QueryRepo 的最小测试替身。
type fakeQueryRepo struct {
	items []Record
	total int64
}

func (f *fakeQueryRepo) ListRecords(context.Context, ListFilter) ([]Record, int64, error) {
	return f.items, f.total, nil
}

func (f *fakeQueryRepo) GetByKey(_ context.Context, key string) (*Record, error) {
	for i := range f.items {
		if f.items[i].DecisionKey == key {
			return &f.items[i], nil
		}
	}
	return nil, nil
}

// TestQueryUsecase_ListNormalizesBeforeRepo 验证 usecase 在调用存储前收敛分页。
func TestQueryUsecase_ListNormalizesBeforeRepo(t *testing.T) {
	repo := &fakeQueryRepo{items: []Record{{DecisionKey: "dk-1"}}, total: 1}
	u := NewQueryUsecase(repo)
	items, total, err := u.List(context.Background(), ListFilter{Page: -1, PageSize: 9999})
	if err != nil || total != 1 || len(items) != 1 {
		t.Fatalf("List = (%v,%d,%v)", items, total, err)
	}
	rec, err := u.Get(context.Background(), "dk-1")
	if err != nil || rec == nil || rec.DecisionKey != "dk-1" {
		t.Fatalf("Get = (%v,%v)", rec, err)
	}
}
