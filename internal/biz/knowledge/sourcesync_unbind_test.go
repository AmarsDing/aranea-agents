// sourcesync_unbind_test.go —— G-38 结转 A1（W2 挂账收口）：带外删档反扫钉阵。
// 口径：DeleteDocument(Mirror) 删档成功后 best-effort 回调 SourceItemUnbinder；
// 未接线零旁效应；回调失败仅 Warn 不阻断已成立的删除。
package knowledge

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/pkg/apierror"
)

type fakeSourceItemUnbinder struct {
	calls []string
	rows  int
	err   error
}

func (f *fakeSourceItemUnbinder) UnbindItemsByDocumentID(_ context.Context, documentID string) (int, error) {
	f.calls = append(f.calls, documentID)
	return f.rows, f.err
}

// 钉①：成功删档触发反扫（docID 透传）。
func TestUsecase_DeleteDocumentMirror_UnbindsSourceItems(t *testing.T) {
	mr := noOpMockRepo()
	u := NewUsecaseFromRepo(mr)
	f := &fakeSourceItemUnbinder{rows: 2}
	u.SetSourceItemUnbinder(f)
	if err := u.DeleteDocumentMirror(context.Background(), "doc-1"); err != nil {
		t.Fatalf("delete mirror: %v", err)
	}
	if len(f.calls) != 1 || f.calls[0] != "doc-1" {
		t.Fatalf("unbinder calls = %v, want [doc-1]", f.calls)
	}
}

// 钉②：未接线 unbinder 时删除路径零旁效应（无 nil dereference、无恐慌）。
func TestUsecase_DeleteDocumentMirror_NilUnbinderSafe(t *testing.T) {
	mr := noOpMockRepo()
	u := NewUsecaseFromRepo(mr)
	if err := u.DeleteDocumentMirror(context.Background(), "doc-1"); err != nil {
		t.Fatalf("delete mirror with nil unbinder: %v", err)
	}
}

// 钉③：回调失败仅 Warn——删除语义已成立，错误不外逸（下轮 connector 兜底）。
func TestUsecase_DeleteDocumentMirror_UnbindErrorSwallowed(t *testing.T) {
	mr := noOpMockRepo()
	u := NewUsecaseFromRepo(mr)
	f := &fakeSourceItemUnbinder{err: errors.New("pg down")}
	u.SetSourceItemUnbinder(f)
	if err := u.DeleteDocumentMirror(context.Background(), "doc-1"); err != nil {
		t.Fatalf("unbind failure must not fail delete, got %v", err)
	}
	if len(f.calls) != 1 {
		t.Fatalf("unbinder calls = %v, want 1 call", f.calls)
	}
}

// 钉④：删除失败时不触发反扫（文档仍在，绑定不该清）。
func TestUsecase_DeleteDocumentMirror_DeleteFailureSkipsUnbind(t *testing.T) {
	mr := noOpMockRepo()
	mr.docDeleteFn = func(_ context.Context, _ string) error {
		return apierror.Internal(apierror.DomainKnowledge, "storage boom")
	}
	u := NewUsecaseFromRepo(mr)
	f := &fakeSourceItemUnbinder{}
	u.SetSourceItemUnbinder(f)
	if err := u.DeleteDocumentMirror(context.Background(), "doc-1"); err == nil {
		t.Fatal("expected delete failure")
	}
	if len(f.calls) != 0 {
		t.Fatalf("unbinder must not fire on delete failure, calls = %v", f.calls)
	}
}

// 钉⑤：DeleteDocument 全链（local 回滚准备 + mirror）同样触发一次反扫——
// 双入口经同一钩点，不重复回调。
func TestUsecase_DeleteDocument_UnbindsViaMirrorChokePoint(t *testing.T) {
	mr := noOpMockRepo()
	// local 回滚准备：文档无 rel_path → 直落 nil rollback（无需 filer）。
	u := NewUsecaseFromRepo(mr)
	f := &fakeSourceItemUnbinder{rows: 3}
	u.SetSourceItemUnbinder(f)
	if err := u.DeleteDocument(context.Background(), "doc-9"); err != nil {
		t.Fatalf("delete document: %v", err)
	}
	if len(f.calls) != 1 || f.calls[0] != "doc-9" {
		t.Fatalf("unbinder calls = %v, want exactly one [doc-9]", f.calls)
	}
}
