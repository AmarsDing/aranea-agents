package knowledge

import (
	"context"
	"strings"
	"testing"
	"time"

	"aranea-agents/pkg/apierror"
)

func TestSplitWriteBackFacts_ReviewBand(t *testing.T) {
	pass, review := SplitWriteBackFacts([]WriteBackFact{
		{FactID: "1", Statement: "用户偏好深色模式界面", FactKind: "preference", Confidence: 0.92},
		{FactID: "2", Statement: "用户可能喜欢咖啡但也说不准", FactKind: "preference", Confidence: 0.72},
		{FactID: "3", Statement: "今天开了三次会讨论排期", FactKind: "fact", Confidence: 0.99},
		{FactID: "4", Statement: "短", FactKind: "preference", Confidence: 0.7},
	})
	if len(pass) != 1 || pass[0].FactID != "1" {
		t.Fatalf("pass=%+v", pass)
	}
	if len(review) != 1 || review[0].FactID != "2" {
		t.Fatalf("review=%+v", review)
	}
}

func TestParsePendingWriteBackItems(t *testing.T) {
	body := writeBackPendingHeader() + FormatPendingAppendix(WriteBackInput{
		SessionID: "s1", AgentID: "ag-1", UserID: "u-1",
	}, []WriteBackFact{{
		FactID: "fid-9", Statement: "用户可能喜欢咖啡但也说不准", FactKind: "preference", Confidence: 0.72, SourceKind: "auto_memory",
	}})
	items := ParsePendingWriteBackItems(body)
	if len(items) != 1 {
		t.Fatalf("len=%d body=%q", len(items), body)
	}
	it := items[0]
	if it.Fact.FactID != "fid-9" || it.AgentID != "ag-1" || it.SessionID != "s1" {
		t.Fatalf("item=%+v", it)
	}
	if !strings.Contains(it.Fact.Statement, "咖啡") {
		t.Fatalf("statement=%q", it.Fact.Statement)
	}
	if it.Fact.Confidence < 0.7 || it.Fact.FactKind != "preference" {
		t.Fatalf("fields=%+v", it.Fact)
	}
}

func TestUsecase_EnqueueAndApplyPending(t *testing.T) {
	m := noOpMockRepo()
	docs := map[string]Document{}
	m.collListFn = func(_ context.Context, ws string, _, _ int) ([]Collection, int, error) {
		return []Collection{{ID: "team-1", Name: WriteBackCollectionName, VaultBackend: VaultBackendTeam, Workspace: ws}}, 1, nil
	}
	m.collGetFn = func(_ context.Context, id string) (Collection, error) {
		return Collection{ID: id, Name: WriteBackCollectionName, VaultBackend: VaultBackendTeam, Workspace: "ws-1"}, nil
	}
	m.docGetByRelFn = func(_ context.Context, _, rel string) (Document, error) {
		for _, d := range docs {
			if d.RelPath == rel {
				return d, nil
			}
		}
		return Document{}, apierror.NotFound("KNOWLEDGE", "missing")
	}
	m.docCreateFn = func(_ context.Context, d Document) (Document, error) {
		d.ID = "id-" + d.RelPath
		docs[d.ID] = d
		return d, nil
	}
	m.docContentFn = func(_ context.Context, id, contentText string, _ bool) error {
		d := docs[id]
		d.ContentText = contentText
		docs[id] = d
		return nil
	}
	u := NewUsecaseFromRepo(m)
	in := WriteBackInput{
		Workspace: "ws-1",
		SessionID: "s1",
		AgentID:   "ag-1",
		Facts: []WriteBackFact{{
			FactID: "fid-low", Statement: "用户可能喜欢咖啡但也说不准", FactKind: "preference", Confidence: 0.72,
		}},
	}
	res, err := u.EnqueueWriteBackReview(context.Background(), in)
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if res.Appended < 1 {
		t.Fatalf("enqueue appended=%d", res.Appended)
	}
	items, err := u.ListPendingWriteBack(context.Background(), "team-1")
	if err != nil || len(items) != 1 {
		t.Fatalf("list: %v items=%+v", err, items)
	}
	applied, err := u.ApplyPendingWriteBack(context.Background(), "team-1", []string{"fid-low"})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if applied.Appended < 1 {
		t.Fatalf("apply appended=%d docs=%+v", applied.Appended, docs)
	}
	dayRel := WriteBackRelPath(time.Now())
	var daily Document
	for _, d := range docs {
		if d.RelPath == dayRel {
			daily = d
		}
	}
	if !strings.Contains(daily.ContentText, "fid-low") {
		t.Fatalf("daily missing fact: %+v", docs)
	}
	left, _ := u.ListPendingWriteBack(context.Background(), "team-1")
	if len(left) != 0 {
		t.Fatalf("pending leftover=%+v", left)
	}
}
