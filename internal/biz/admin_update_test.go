package biz

import (
	"context"
	"errors"
	"strconv"
	"testing"

	"aranea-agents/pkg/loggateway"
)

// 评审 A4（2026-11-09）：UpdateAdmin 角色信号（access/roles）变更必须先吊销
// 会话（tv 递增 + 广播）再落库——否则旧会话凭旧角色 claim 继续存活到自然过期。
// 本组用例钉住：access 变更 → 吊销先于写入；纯资料更新 → 不吊销；
// roles 集合变更 → 吊销（顺序不敏感）；吊销失败/未接线 → 不写库、如实报错。

// updateOrderRevoker 把吊销调用顺序记录到共享 seq。
type updateOrderRevoker struct {
	seq *[]string
}

func (o *updateOrderRevoker) RevokeSessions(_ context.Context, userID int64) (int64, error) {
	*o.seq = append(*o.seq, "revoke:"+strconv.FormatInt(userID, 10))
	return 2, nil
}

// orderRecordingWriter 在真实 writer 调用时记录顺序（吊销必须先于写入）。
type orderRecordingWriter struct {
	AdminWriter
	seq *[]string
	no  int
}

func (o *orderRecordingWriter) UpdateAdmin(ctx context.Context, a *Admin) (*Admin, error) {
	*o.seq = append(*o.seq, "write:"+strconv.FormatInt(a.ID, 10))
	o.no++
	return o.AdminWriter.UpdateAdmin(ctx, a)
}

// TestUpdateAdminAccessChangeRevokesFirst access 变更：吊销必须先于写入。
func TestUpdateAdminAccessChangeRevokesFirst(t *testing.T) {
	var seq []string
	store := &fakeAdminStore{byID: map[int64]*Admin{
		7: {ID: 7, Name: "u", Access: "user", Roles: []string{"user"}, Status: AdminStatusActive},
	}}
	wrapped := &orderRecordingWriter{AdminWriter: store, seq: &seq}
	uc := NewAdminUsecase(store, wrapped, loggateway.NewNoop())
	uc.SetSessionRevoker(&updateOrderRevoker{seq: &seq})

	in := &Admin{ID: 7, Name: "u", Access: "admin"} // 提权
	if _, err := uc.UpdateAdmin(context.Background(), in); err != nil {
		t.Fatalf("UpdateAdmin: %v", err)
	}
	if wrapped.no != 1 {
		t.Fatalf("writer calls = %d, want 1", wrapped.no)
	}
	if len(seq) != 2 || seq[0] != "revoke:7" || seq[1] != "write:7" {
		t.Fatalf("call order = %v, want [revoke:7 write:7]", seq)
	}
}

// TestUpdateAdminProfileOnlyNoRevoke 纯资料更新（姓名/头像）不触碰角色信号：
// 不吊销、直接落库。
func TestUpdateAdminProfileOnlyNoRevoke(t *testing.T) {
	store := &fakeAdminStore{byID: map[int64]*Admin{
		7: {ID: 7, Name: "old", Access: "user", Roles: []string{"user"}, Status: AdminStatusActive},
	}}
	uc := newAdminUCForTest(store)
	rv := &fakeRevoker{}
	uc.SetSessionRevoker(rv)

	in := &Admin{ID: 7, Name: "new-name", Access: "user", Avatar: "a.png"} // service 路径：Roles nil
	if _, err := uc.UpdateAdmin(context.Background(), in); err != nil {
		t.Fatalf("UpdateAdmin: %v", err)
	}
	if rv.no != 0 {
		t.Fatalf("revoker calls = %d, want 0 (profile-only update)", rv.no)
	}
	if store.updateNo != 1 {
		t.Fatalf("update calls = %d, want 1", store.updateNo)
	}
}

// TestUpdateAdminRolesChangeRevokes roles 集合变更 → 吊销；同集不同顺序不吊销。
func TestUpdateAdminRolesChangeRevokes(t *testing.T) {
	cases := []struct {
		name    string
		old     []string
		in      []string
		revoked bool
	}{
		{name: "add_role", old: []string{"user"}, in: []string{"user", "auditor"}, revoked: true},
		{name: "remove_role", old: []string{"user", "auditor"}, in: []string{"user"}, revoked: true},
		{name: "same_set_reordered", old: []string{"user", "auditor"}, in: []string{"auditor", "user"}, revoked: false},
		{name: "nil_roles_unchanged", old: []string{"user"}, in: nil, revoked: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := &fakeAdminStore{byID: map[int64]*Admin{
				7: {ID: 7, Access: "user", Roles: tc.old, Status: AdminStatusActive},
			}}
			uc := newAdminUCForTest(store)
			rv := &fakeRevoker{}
			uc.SetSessionRevoker(rv)

			in := &Admin{ID: 7, Access: "user", Roles: tc.in}
			if _, err := uc.UpdateAdmin(context.Background(), in); err != nil {
				t.Fatalf("UpdateAdmin: %v", err)
			}
			if got := rv.no > 0; got != tc.revoked {
				t.Fatalf("revoked = %v, want %v (revoker calls=%d)", got, tc.revoked, rv.no)
			}
			if store.updateNo != 1 {
				t.Fatalf("update calls = %d, want 1", store.updateNo)
			}
		})
	}
}

// TestUpdateAdminRevokeFailureAbortsWrite 吊销失败（revoker 内部错误）时不得
// 写库：绝不留下「角色已改但旧会话仍活」的窗口。
func TestUpdateAdminRevokeFailureAbortsWrite(t *testing.T) {
	store := &fakeAdminStore{byID: map[int64]*Admin{
		7: {ID: 7, Access: "user", Roles: []string{"user"}, Status: AdminStatusActive},
	}}
	uc := newAdminUCForTest(store)
	uc.SetSessionRevoker(&fakeRevoker{err: errors.New("redis down")})

	in := &Admin{ID: 7, Access: "admin"} // 提权
	if _, err := uc.UpdateAdmin(context.Background(), in); err == nil {
		t.Fatal("expected revoke error to surface")
	}
	if store.updateNo != 0 {
		t.Fatalf("update calls = %d, want 0 (revoke precedes write)", store.updateNo)
	}
	if store.byID[7].Access != "user" {
		t.Fatalf("stored access mutated to %q, want user", store.byID[7].Access)
	}
}

// TestUpdateAdminRevokerUnwiredHonestFailure 角色变更且 revoker 未接线：
// 诚实失败且不动数据（宁可角色变更不可用，也不静默假吊销）。
func TestUpdateAdminRevokerUnwiredHonestFailure(t *testing.T) {
	store := &fakeAdminStore{byID: map[int64]*Admin{
		7: {ID: 7, Access: "user", Roles: []string{"user"}, Status: AdminStatusActive},
	}}
	uc := newAdminUCForTest(store) // 未 SetSessionRevoker

	in := &Admin{ID: 7, Access: "admin"}
	if _, err := uc.UpdateAdmin(context.Background(), in); err == nil {
		t.Fatal("role change without revoker must fail honestly")
	}
	if store.updateNo != 0 {
		t.Fatalf("update calls = %d, want 0", store.updateNo)
	}
	if store.byID[7].Access != "user" {
		t.Fatalf("stored access mutated to %q, want user", store.byID[7].Access)
	}
}
