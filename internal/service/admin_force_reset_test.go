package service_test

import (
	"context"
	"errors"
	"testing"

	adminv1 "aranea-agents/api/kratos/admin/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/service"
	"aranea-agents/pkg/loggateway"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

// fakeAdminRW 是 UpdateAdmin 测试用的内存 AdminReader/Writer。
type fakeAdminRW struct {
	byID      map[int64]*biz.Admin
	updated   *biz.Admin
	updateErr error
}

func (f *fakeAdminRW) FindByID(_ context.Context, id int64) (*biz.Admin, error) {
	if a, ok := f.byID[id]; ok {
		cp := *a
		return &cp, nil
	}
	return nil, biz.ErrAdminNotFound
}
func (f *fakeAdminRW) FindByName(context.Context, string) (*biz.Admin, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeAdminRW) FindByEmail(context.Context, string) (*biz.Admin, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeAdminRW) ListAdmins(context.Context, ...biz.ListOption) ([]*biz.Admin, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeAdminRW) CreateAdmin(context.Context, *biz.Admin) (*biz.Admin, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeAdminRW) UpdateAdmin(_ context.Context, a *biz.Admin) (*biz.Admin, error) {
	if f.updateErr != nil {
		return nil, f.updateErr
	}
	f.updated = a
	f.byID[a.ID] = a
	return a, nil
}
func (f *fakeAdminRW) DeleteAdmin(context.Context, int64) error {
	return errors.New("not implemented")
}

type fakeRevoker struct {
	calls  int
	userID int64
	err    error
}

func (f *fakeRevoker) RevokeSessions(_ context.Context, userID int64) (int64, error) {
	f.calls++
	f.userID = userID
	return int64(f.calls), f.err
}

func newAdminSvcForForceReset(store *fakeAdminRW, revoker *fakeRevoker) *service.AdminService {
	uc := biz.NewAdminUsecase(store, store, loggateway.NewNoop())
	if revoker != nil {
		uc.SetSessionRevoker(revoker)
	}
	return service.NewAdminService(uc, loggateway.NewNoop())
}

// adminCtx 复用 skill_test.go 的既有定义(同包)。

func TestUpdateAdmin_PreservesForceResetWhenMaskOmitsIt(t *testing.T) {
	// 管理台编辑普通字段不得清掉 force_reset(回归守护:此前 data 层不持久化,
	// 服务层构造也丢字段,双缺口叠加会让改密门失效)。
	store := &fakeAdminRW{byID: map[int64]*biz.Admin{
		7: {ID: 7, Name: "wb-user", Access: "user", ForceReset: true},
	}}
	revoker := &fakeRevoker{}
	svc := newAdminSvcForForceReset(store, revoker)

	_, err := svc.UpdateAdmin(adminCtx(), &adminv1.UpdateAdminRequest{
		Admin:      &adminv1.Admin{Id: 7, Name: "wb-user-renamed"},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"name"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if store.updated == nil || !store.updated.ForceReset {
		t.Fatalf("mask 未触及 force_reset 时必须保持原值 true,实际 %+v", store.updated)
	}
	if revoker.calls != 0 {
		t.Fatalf("非置位变更不应吊销会话,吊销 %d 次", revoker.calls)
	}
}

func TestUpdateAdmin_SetForceResetRevokesSessionsFirst(t *testing.T) {
	store := &fakeAdminRW{byID: map[int64]*biz.Admin{
		7: {ID: 7, Name: "u", Access: "user", ForceReset: false},
	}}
	revoker := &fakeRevoker{}
	svc := newAdminSvcForForceReset(store, revoker)

	_, err := svc.UpdateAdmin(adminCtx(), &adminv1.UpdateAdminRequest{
		Admin:      &adminv1.Admin{Id: 7, ForceReset: true},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"force_reset"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if revoker.calls != 1 || revoker.userID != 7 {
		t.Fatalf("置位须先吊销目标会话,calls=%d userID=%d", revoker.calls, revoker.userID)
	}
	if store.updated == nil || !store.updated.ForceReset {
		t.Fatalf("置位未落库:%+v", store.updated)
	}
}

func TestUpdateAdmin_SetForceResetFailsClosedWhenRevokerUnavailable(t *testing.T) {
	store := &fakeAdminRW{byID: map[int64]*biz.Admin{
		7: {ID: 7, Name: "u", Access: "user", ForceReset: false},
	}}
	svc := newAdminSvcForForceReset(store, nil) // 未接 revoker

	_, err := svc.UpdateAdmin(adminCtx(), &adminv1.UpdateAdminRequest{
		Admin:      &adminv1.Admin{Id: 7, ForceReset: true},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"force_reset"}},
	})
	if err == nil {
		t.Fatal("吊销不可用时不应静默置位(安全场景 fail-closed)")
	}
	if store.updated != nil {
		t.Fatal("吊销失败即不落库,不容半态")
	}
}

func TestUpdateAdmin_ClearForceResetDoesNotRevoke(t *testing.T) {
	store := &fakeAdminRW{byID: map[int64]*biz.Admin{
		7: {ID: 7, Name: "u", Access: "user", ForceReset: true},
	}}
	revoker := &fakeRevoker{}
	svc := newAdminSvcForForceReset(store, revoker)

	_, err := svc.UpdateAdmin(adminCtx(), &adminv1.UpdateAdminRequest{
		Admin:      &adminv1.Admin{Id: 7, ForceReset: false},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"force_reset"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if store.updated == nil || store.updated.ForceReset {
		t.Fatalf("清除未生效:%+v", store.updated)
	}
	if revoker.calls != 0 {
		t.Fatalf("清除不吊销(旧 fr=true 会话随重新登录自然失效),吊销 %d 次", revoker.calls)
	}
}
