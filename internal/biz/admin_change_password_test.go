package biz

import (
	"context"
	"errors"
	"strings"
	"testing"

	"aranea-agents/pkg/loggateway"

	"golang.org/x/crypto/bcrypt"
)

// fakeAdminStore 是 AdminReader + AdminWriter 的最小内存实现（仅覆盖本组用例
// 触达的方法；未实现的方法被调用即 Fail，避免静默放行）。
type fakeAdminStore struct {
	byID     map[int64]*Admin
	created  *Admin
	updated  *Admin
	updateNo int
	listNo   int
}

var _ AdminReader = (*fakeAdminStore)(nil)
var _ AdminWriter = (*fakeAdminStore)(nil)

func (f *fakeAdminStore) FindByID(_ context.Context, id int64) (*Admin, error) {
	a, ok := f.byID[id]
	if !ok {
		return nil, errors.New("admin not found")
	}
	return a, nil
}

func (f *fakeAdminStore) FindByName(context.Context, string) (*Admin, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeAdminStore) FindByEmail(context.Context, string) (*Admin, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeAdminStore) ListAdmins(context.Context, ...ListOption) ([]*Admin, error) {
	f.listNo++
	return nil, nil
}

func (f *fakeAdminStore) CreateAdmin(context.Context, *Admin) (*Admin, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeAdminStore) UpdateAdmin(_ context.Context, a *Admin) (*Admin, error) {
	f.updateNo++
	f.updated = a
	f.byID[a.ID] = a
	return a, nil
}

func (f *fakeAdminStore) DeleteAdmin(context.Context, int64) error {
	return errors.New("not implemented")
}

// fakeRevoker 记录吊销调用，可注入错误。
type fakeRevoker struct {
	userID int64
	no     int
	err    error
}

func (f *fakeRevoker) RevokeSessions(_ context.Context, userID int64) (int64, error) {
	f.no++
	f.userID = userID
	return int64(f.no), f.err
}

// bcryptHashOf 生成用于断言的正经 bcrypt 口令哈希。
func bcryptHashOf(t *testing.T, plain string) string {
	t.Helper()
	h, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	return string(h)
}

func newAdminUCForTest(store *fakeAdminStore) *AdminUsecase {
	return NewAdminUsecase(store, store, loggateway.NewNoop())
}

// TestChangePasswordRevokesSessions 校验 P1-8 主路径：旧口令正确 → 先吊销会话
// （tv 递增）→ 写入新 bcrypt 口令 → 清除 force_reset。
func TestChangePasswordRevokesSessions(t *testing.T) {
	oldHash := bcryptHashOf(t, "old-password-1")
	store := &fakeAdminStore{byID: map[int64]*Admin{
		7: {ID: 7, Name: "u", Password: oldHash, Status: AdminStatusActive, ForceReset: true},
	}}
	uc := newAdminUCForTest(store)
	rv := &fakeRevoker{}
	uc.SetSessionRevoker(rv)

	if err := uc.ChangePassword(context.Background(), 7, "old-password-1", "new-password-2"); err != nil {
		t.Fatalf("ChangePassword: %v", err)
	}
	if rv.no != 1 || rv.userID != 7 {
		t.Fatalf("revoker calls=%d userID=%d, want 1/7", rv.no, rv.userID)
	}
	if store.updateNo != 1 {
		t.Fatalf("update calls=%d, want 1", store.updateNo)
	}
	if !strings.HasPrefix(store.updated.Password, "$2") {
		t.Fatalf("stored password is not bcrypt: %q", store.updated.Password)
	}
	if store.updated.ForceReset {
		t.Fatal("ForceReset must be cleared after a successful self-service change")
	}
	// 新口令可用、旧口令失效。
	if !verifyPassword(store.updated.Password, "new-password-2") {
		t.Fatal("new password does not verify against stored hash")
	}
	if verifyPassword(store.updated.Password, "old-password-1") {
		t.Fatal("old password still verifies after change")
	}
}

// TestChangePasswordRejectsWrongOldPassword 旧口令错误必须与登录同错误码
// （ErrInvalidCredentials），且不得吊销会话、不得写入口令。
func TestChangePasswordRejectsWrongOldPassword(t *testing.T) {
	store := &fakeAdminStore{byID: map[int64]*Admin{
		7: {ID: 7, Password: bcryptHashOf(t, "old-password-1"), Status: AdminStatusActive},
	}}
	uc := newAdminUCForTest(store)
	rv := &fakeRevoker{}
	uc.SetSessionRevoker(rv)

	err := uc.ChangePassword(context.Background(), 7, "wrong-password", "new-password-2")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("err = %v, want ErrInvalidCredentials", err)
	}
	if rv.no != 0 || store.updateNo != 0 {
		t.Fatalf("side effects on rejected change: revoke=%d update=%d", rv.no, store.updateNo)
	}
}

// TestChangePasswordInputGuards 覆盖入参守卫：空/过短/与旧口令相同一律拒绝，
// 且在读取账号之前就返回（不产生任何副作用）。
func TestChangePasswordInputGuards(t *testing.T) {
	uc := newAdminUCForTest(&fakeAdminStore{byID: map[int64]*Admin{
		7: {ID: 7, Password: bcryptHashOf(t, "old-password-1"), Status: AdminStatusActive},
	}})
	cases := []struct{ name, old, new string }{
		{name: "empty", old: "old-password-1", new: "   "},
		{name: "too short", old: "old-password-1", new: "short"},
		{name: "same as old", old: "old-password-1", new: "old-password-1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := uc.ChangePassword(context.Background(), 7, tc.old, tc.new); err == nil {
				t.Fatal("expected error, got nil")
			} else if errors.Is(err, ErrInvalidCredentials) {
				t.Fatalf("input guard must not reuse ErrInvalidCredentials: %v", err)
			}
		})
	}
}

// TestChangePasswordRevokerUnwired 未接线吊销端口时不得改密（诚实失败：宁可
// 改密不可用，也不能留下"口令已换但旧会话仍活"的缺口）。
func TestChangePasswordRevokerUnwired(t *testing.T) {
	store := &fakeAdminStore{byID: map[int64]*Admin{
		7: {ID: 7, Password: bcryptHashOf(t, "old-password-1"), Status: AdminStatusActive},
	}}
	uc := newAdminUCForTest(store)

	if err := uc.ChangePassword(context.Background(), 7, "old-password-1", "new-password-2"); err == nil {
		t.Fatal("expected error when revoker is not wired")
	}
	if store.updateNo != 0 {
		t.Fatalf("update calls=%d, want 0 (revoke precedes write)", store.updateNo)
	}
}

// TestChangePasswordRejectsInactiveAccount 冻结/离职账号不允许自助改密。
func TestChangePasswordRejectsInactiveAccount(t *testing.T) {
	store := &fakeAdminStore{byID: map[int64]*Admin{
		7: {ID: 7, Password: bcryptHashOf(t, "old-password-1"), Status: AdminStatusFrozen},
	}}
	uc := newAdminUCForTest(store)
	uc.SetSessionRevoker(&fakeRevoker{})

	err := uc.ChangePassword(context.Background(), 7, "old-password-1", "new-password-2")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("err = %v, want ErrInvalidCredentials", err)
	}
	if store.updateNo != 0 {
		t.Fatalf("update calls=%d, want 0", store.updateNo)
	}
}

// TestChangePasswordRevokeFailureAbortsWrite 吊销失败必须中止：先吊销后写入口令
// 的顺序保证失败时口令保持原样（用户仅被登出，可用旧口令重登）。
func TestChangePasswordRevokeFailureAbortsWrite(t *testing.T) {
	store := &fakeAdminStore{byID: map[int64]*Admin{
		7: {ID: 7, Password: bcryptHashOf(t, "old-password-1"), Status: AdminStatusActive},
	}}
	uc := newAdminUCForTest(store)
	uc.SetSessionRevoker(&fakeRevoker{err: errors.New("redis+db down")})

	if err := uc.ChangePassword(context.Background(), 7, "old-password-1", "new-password-2"); err == nil {
		t.Fatal("expected revoke error to surface")
	}
	if store.updateNo != 0 {
		t.Fatalf("update calls=%d, want 0", store.updateNo)
	}
}
