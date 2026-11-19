package biz

import (
	"context"
	"errors"
	"strconv"
	"testing"

	"aranea-agents/pkg/loggateway"
)

// 评审 H1（2026-11-09）：DeleteAdmin 先吊销会话再删号——否则被删用户的 WS
// 连接最长存活 7 天无任何踢出手段。本组用例钉住三件事：吊销先于删除、
// 吊销失败则不删、revoker 未接线时诚实失败且不动数据。

// orderRevoker 记录吊销调用顺序到共享 seq。
type orderRevoker struct {
	seq *[]string
}

func (o *orderRevoker) RevokeSessions(_ context.Context, userID int64) (int64, error) {
	*o.seq = append(*o.seq, "revoke:"+strconv.FormatInt(userID, 10))
	return 2, nil
}

// TestDeleteAdminRevokesSessionsFirst 吊销必须先于删除（顺序断言）。
func TestDeleteAdminRevokesSessionsFirst(t *testing.T) {
	var seq []string
	store := &fakeAdminStore{byID: map[int64]*Admin{7: {ID: 7}}}
	store.deleteHook = func(int64) { seq = append(seq, "delete") }

	uc := NewAdminUsecase(store, store, loggateway.NewNoop())
	uc.SetSessionRevoker(&orderRevoker{seq: &seq})

	if err := uc.DeleteAdmin(context.Background(), 7); err != nil {
		t.Fatalf("DeleteAdmin: %v", err)
	}
	if store.deleteNo != 1 {
		t.Fatalf("DeleteAdmin calls = %d, want 1", store.deleteNo)
	}
	if _, ok := store.byID[7]; ok {
		t.Fatal("user 7 should be removed from store")
	}
	if len(seq) != 2 || seq[0] != "revoke:7" || seq[1] != "delete" {
		t.Fatalf("call order = %v, want [revoke:7 delete]", seq)
	}
}

// TestDeleteAdminRevokerFailureKeepsUser 吊销失败（revoker 内部错误或未接线）
// 时不得删号：绝不留下「账号已删但会话仍活」的窗口。
func TestDeleteAdminRevokerFailureKeepsUser(t *testing.T) {
	// revoker 返回错误
	store1 := &fakeAdminStore{byID: map[int64]*Admin{7: {ID: 7}}}
	uc1 := NewAdminUsecase(store1, store1, loggateway.NewNoop())
	uc1.SetSessionRevoker(&fakeRevoker{err: errors.New("redis down")})
	if err := uc1.DeleteAdmin(context.Background(), 7); err == nil {
		t.Fatal("DeleteAdmin with failing revoker must return error")
	}
	if store1.deleteNo != 0 {
		t.Fatalf("store delete must not run on revoke failure, got %d calls", store1.deleteNo)
	}
	if _, ok := store1.byID[7]; !ok {
		t.Fatal("user 7 must survive failed revocation")
	}

	// revoker 未接线（诚实失败，绝不静默假吊销）
	store2 := &fakeAdminStore{byID: map[int64]*Admin{8: {ID: 8}}}
	uc2 := NewAdminUsecase(store2, store2, loggateway.NewNoop())
	if err := uc2.DeleteAdmin(context.Background(), 8); err == nil {
		t.Fatal("DeleteAdmin without revoker must fail honestly")
	}
	if store2.deleteNo != 0 {
		t.Fatalf("store delete must not run without revoker, got %d calls", store2.deleteNo)
	}
}
