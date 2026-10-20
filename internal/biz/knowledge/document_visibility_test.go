package knowledge

import (
	"context"
	"testing"

	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/auth"
)

func TestDocumentVisibleTo(t *testing.T) {
	t.Parallel()
	private := Document{ID: "d1", Visibility: DocVisibilityPrivate, OwnerUserID: "7"}
	shared := Document{ID: "d2", Visibility: DocVisibilityCollection, OwnerUserID: "7"}
	legacy := Document{ID: "d3"}

	sys := workspace.WithSystemWorkspace(context.Background())
	if !DocumentVisibleTo(sys, private) {
		t.Fatal("system must see private docs")
	}

	anon := context.Background()
	if DocumentVisibleTo(anon, private) {
		t.Fatal("anonymous must not see private docs")
	}
	if !DocumentVisibleTo(anon, shared) || !DocumentVisibleTo(anon, legacy) {
		t.Fatal("anonymous must see collection/legacy docs")
	}

	owner := auth.NewContext(context.Background(), &auth.Auth{UserID: 7})
	if !DocumentVisibleTo(owner, private) {
		t.Fatal("owner must see own private doc")
	}
	other := auth.NewContext(context.Background(), &auth.Auth{UserID: 8})
	if DocumentVisibleTo(other, private) {
		t.Fatal("non-owner must not see private doc")
	}
}

// G-08 shared 档谓词：属主恒可见；org scope ∩ shared_with ≠ ∅ 可见（专项 Agent
// 场景：无 auth uid 但带 scope）；人类无 scope 仅属主可见；未知档位拒绝。
func TestDocumentVisibleTo_Shared(t *testing.T) {
	t.Parallel()
	doc := Document{ID: "d1", Visibility: DocVisibilityShared, OwnerUserID: "7", SharedWith: []string{"dept-a", "pos-1"}}

	sys := workspace.WithSystemWorkspace(context.Background())
	if !DocumentVisibleTo(sys, doc) {
		t.Fatal("system must see shared docs")
	}
	owner := auth.NewContext(context.Background(), &auth.Auth{UserID: 7})
	if !DocumentVisibleTo(owner, doc) {
		t.Fatal("owner must see own shared doc")
	}
	// 人类非属主（无 scope）不可见——语义同 private。
	other := auth.NewContext(context.Background(), &auth.Auth{UserID: 8})
	if DocumentVisibleTo(other, doc) {
		t.Fatal("non-owner without org scope must not see shared doc")
	}
	if DocumentVisibleTo(context.Background(), doc) {
		t.Fatal("anonymous without org scope must not see shared doc")
	}
	// 命中部门 / 命中岗位。
	deptCtx := WithOrgScope(context.Background(), []string{"dept-a", "pos-9"})
	if !DocumentVisibleTo(deptCtx, doc) {
		t.Fatal("agent in listed department must see shared doc")
	}
	posCtx := WithOrgScope(context.Background(), []string{"pos-1"})
	if !DocumentVisibleTo(posCtx, doc) {
		t.Fatal("agent at listed position must see shared doc")
	}
	// 带身份但未命中名单的 Agent 也不可见（scope 不扩 owner 腿之外的域）。
	missCtx := WithOrgScope(auth.NewContext(context.Background(), &auth.Auth{UserID: 8}), []string{"dept-b"})
	if DocumentVisibleTo(missCtx, doc) {
		t.Fatal("agent outside the share list must not see shared doc")
	}
	// 空名单 / 空 scope fail-closed。
	if DocumentVisibleTo(deptCtx, Document{ID: "d2", Visibility: DocVisibilityShared}) {
		t.Fatal("shared doc with empty list must be owner-only")
	}
	// 未知档位 deny-by-default。
	if DocumentVisibleTo(owner, Document{ID: "d3", Visibility: "weird"}) {
		t.Fatal("unknown visibility tier must be denied")
	}
}

func TestUpdateDocumentVisibility_RequiresSignInForPrivate(t *testing.T) {
	t.Parallel()
	docs := map[string]Document{
		"d1": {ID: "d1", Visibility: DocVisibilityCollection},
	}
	u := NewUsecaseFromRepo(&mockRepo{
		docGetFn: func(_ context.Context, id string) (Document, error) {
			return docs[id], nil
		},
	})
	u.SetDocumentACLStore(&stubACL{})
	if _, err := u.UpdateDocumentVisibility(context.Background(), "d1", DocVisibilityPrivate, nil); err == nil {
		t.Fatal("anonymous private mark must fail")
	}
}

func TestUpdateDocumentVisibility_SetsOwner(t *testing.T) {
	t.Parallel()
	docs := map[string]Document{
		"d1": {ID: "d1", Visibility: DocVisibilityCollection},
	}
	u := NewUsecaseFromRepo(&mockRepo{
		docGetFn: func(_ context.Context, id string) (Document, error) {
			return docs[id], nil
		},
	})
	u.SetDocumentACLStore(&stubACL{})
	// G-03：owner 为空的共享资产仅 admin 可收编；收编时 owner 记为 admin 本人。
	ctx := auth.NewContext(context.Background(), &auth.Auth{UserID: 9, Access: "admin"})
	got, err := u.UpdateDocumentVisibility(ctx, "d1", DocVisibilityPrivate, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Visibility != DocVisibilityPrivate || got.OwnerUserID != "9" {
		t.Fatalf("got %+v", got)
	}
}

func TestUpdateDocumentVisibility_NonOwnerForbidden(t *testing.T) {
	t.Parallel()
	docs := map[string]Document{
		"d1": {ID: "d1", Visibility: DocVisibilityCollection, OwnerUserID: "7"},
	}
	u := NewUsecaseFromRepo(&mockRepo{
		docGetFn: func(_ context.Context, id string) (Document, error) {
			return docs[id], nil
		},
	})
	u.SetDocumentACLStore(&stubACL{})
	// 他人（非 owner 非 admin）不得私有化/夺产——无论改 private 还是 collection。
	for _, vis := range []string{DocVisibilityPrivate, DocVisibilityCollection} {
		other := auth.NewContext(context.Background(), &auth.Auth{UserID: 8})
		if _, err := u.UpdateDocumentVisibility(other, "d1", vis, nil); err == nil {
			t.Fatalf("non-owner change to %s must fail", vis)
		}
	}
	// owner 本人可以。
	owner := auth.NewContext(context.Background(), &auth.Auth{UserID: 7})
	if _, err := u.UpdateDocumentVisibility(owner, "d1", DocVisibilityPrivate, nil); err != nil {
		t.Fatalf("owner must be allowed: %v", err)
	}
	// admin 可以，且不改写原 owner（管理员处置不夺产）。
	admin := auth.NewContext(context.Background(), &auth.Auth{UserID: 9, Access: "admin"})
	got, err := u.UpdateDocumentVisibility(admin, "d1", DocVisibilityCollection, nil)
	if err != nil {
		t.Fatalf("admin must be allowed: %v", err)
	}
	if got.OwnerUserID != "7" {
		t.Fatalf("admin change must preserve owner, got %+v", got)
	}
	// system 可以。
	sys := workspace.WithSystemWorkspace(context.Background())
	if _, err := u.UpdateDocumentVisibility(sys, "d1", DocVisibilityCollection, nil); err != nil {
		t.Fatalf("system must be allowed: %v", err)
	}
}

type stubACL struct {
	id, visibility, owner string
	sharedWith            []string
}

func (s *stubACL) UpdateDocumentVisibility(_ context.Context, id, visibility, ownerUserID string, sharedWith []string) error {
	s.id = id
	s.visibility = visibility
	s.owner = ownerUserID
	s.sharedWith = sharedWith
	return nil
}

// ── G-08：shared 档写入校验 ────────────────────────────────────────────────

type stubOrgValidator struct {
	err  error
	seen []string
}

func (v *stubOrgValidator) ValidateSharedOrgNodes(_ context.Context, ids []string) error {
	v.seen = append([]string(nil), ids...)
	return v.err
}

func TestUpdateDocumentVisibility_SharedValidation(t *testing.T) {
	t.Parallel()
	docs := map[string]Document{
		"d1": {ID: "d1", Visibility: DocVisibilityCollection, OwnerUserID: "7"},
	}
	mkUsecase := func(v *stubOrgValidator) (*Usecase, *stubACL) {
		u := NewUsecaseFromRepo(&mockRepo{
			docGetFn: func(_ context.Context, id string) (Document, error) {
				return docs[id], nil
			},
		})
		acl := &stubACL{}
		u.SetDocumentACLStore(acl)
		if v != nil {
			u.SetOrgNodeValidator(v)
		}
		return u, acl
	}
	owner := auth.NewContext(context.Background(), &auth.Auth{UserID: 7})

	// shared + 空名单 → BadRequest。
	u, _ := mkUsecase(&stubOrgValidator{})
	if _, err := u.UpdateDocumentVisibility(owner, "d1", DocVisibilityShared, nil); err == nil {
		t.Fatal("shared with empty list must fail")
	}
	// shared + 未接线 validator → Unavailable。
	uNoV, _ := mkUsecase(nil)
	if _, err := uNoV.UpdateDocumentVisibility(owner, "d1", DocVisibilityShared, []string{"dept-a"}); err == nil {
		t.Fatal("shared without validator must fail")
	}
	// validator 拒绝（如节点为 company 级 / 不存在 / 已软删）→ 透传。
	uBad, _ := mkUsecase(&stubOrgValidator{err: apierror.BadRequest("KNOWLEDGE", "org node is not a department/position")})
	if _, err := uBad.UpdateDocumentVisibility(owner, "d1", DocVisibilityShared, []string{"company-x"}); err == nil {
		t.Fatal("validator rejection must propagate")
	}
	// 合法：名单规范化（去空白去重）后落库，doc 回显。
	v := &stubOrgValidator{}
	u, acl := mkUsecase(v)
	got, err := u.UpdateDocumentVisibility(owner, "d1", DocVisibilityShared, []string{" dept-a ", "pos-1", "dept-a", ""})
	if err != nil {
		t.Fatalf("valid shared write must succeed: %v", err)
	}
	want := []string{"dept-a", "pos-1"}
	if len(v.seen) != len(want) || v.seen[0] != want[0] || v.seen[1] != want[1] {
		t.Fatalf("validator must see normalized list, got %v", v.seen)
	}
	if acl.visibility != DocVisibilityShared || acl.owner != "7" || len(acl.sharedWith) != 2 {
		t.Fatalf("acl write wrong: %+v", acl)
	}
	if got.Visibility != DocVisibilityShared || len(got.SharedWith) != 2 || got.SharedWith[0] != "dept-a" {
		t.Fatalf("doc echo wrong: %+v", got)
	}
	// 切回 collection：名单清空（不留隐藏状态）。
	if _, err := u.UpdateDocumentVisibility(owner, "d1", DocVisibilityCollection, []string{"dept-a"}); err != nil {
		t.Fatal(err)
	}
	if len(acl.sharedWith) != 0 {
		t.Fatalf("switch to collection must clear share list, got %v", acl.sharedWith)
	}
}
