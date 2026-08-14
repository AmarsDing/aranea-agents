package knowledge

import (
	"context"
	"strings"
	"testing"
	"time"

	"aranea-agents/pkg/apierror"
)

func TestFilterWriteBackFacts(t *testing.T) {
	in := []WriteBackFact{
		{FactID: "1", Statement: "用户偏好深色模式界面", FactKind: "preference", Confidence: 0.92},
		{FactID: "2", Statement: "短", FactKind: "preference", Confidence: 0.99},
		{FactID: "3", Statement: "用户可能喜欢咖啡但也说不准", FactKind: "preference", Confidence: 0.7},
		{FactID: "4", Statement: "今天开了三次会讨论排期", FactKind: "fact", Confidence: 0.99},
		{FactID: "5", Statement: "部署必须走灰度且保留回滚开关", FactKind: "constraint", Confidence: 0.9},
	}
	got := FilterWriteBackFacts(in)
	if len(got) != 2 {
		t.Fatalf("len=%d want 2: %+v", len(got), got)
	}
	if got[0].FactID != "1" || got[1].FactID != "5" {
		t.Fatalf("ids = %q %q", got[0].FactID, got[1].FactID)
	}
}

func TestFormatWriteBackAppendix_Provenance(t *testing.T) {
	body := FormatWriteBackAppendix(WriteBackInput{
		SessionID: "sess-1",
		AgentID:   "ag-1",
		UserID:    "u-1",
	}, []WriteBackFact{{
		FactID:     "fid-9",
		Statement:  "用户偏好深色模式界面",
		FactKind:   "preference",
		Confidence: 0.91,
		SourceKind: "auto_memory",
	}})
	for _, need := range []string{
		"## preference",
		"用户偏好深色模式界面",
		"fact_id: `fid-9`",
		"session_id: `sess-1`",
		"agent_id: `ag-1`",
		"confidence: 0.91",
		"source: auto_memory",
	} {
		if !strings.Contains(body, need) {
			t.Errorf("missing %q in %q", need, body)
		}
	}
}

func TestWriteBackRelPath_UTCDay(t *testing.T) {
	now := time.Date(2026, 8, 15, 3, 0, 0, 0, time.UTC)
	if got := WriteBackRelPath(now); got != "inbox/writeback-2026-08-15.md" {
		t.Fatalf("got %q", got)
	}
}

func TestUsecase_WriteBackSessionFacts_CreatesDailyNote(t *testing.T) {
	m := noOpMockRepo()
	var created Document
	m.collListFn = func(_ context.Context, ws string, _, _ int) ([]Collection, int, error) {
		return []Collection{{ID: "team-1", Name: "工程团队", VaultBackend: VaultBackendTeam, Workspace: ws}}, 1, nil
	}
	m.docGetByRelFn = func(_ context.Context, _, _ string) (Document, error) {
		return Document{}, apierror.NotFound("KNOWLEDGE", "missing")
	}
	m.docCreateFn = func(_ context.Context, d Document) (Document, error) {
		d.ID = "doc-wb"
		created = d
		return d, nil
	}
	u := NewUsecaseFromRepo(m)

	res, err := u.WriteBackSessionFacts(context.Background(), WriteBackInput{
		Workspace: "ws-1",
		SessionID: "sess-1",
		AgentID:   "ag-1",
		Facts: []WriteBackFact{{
			FactID:     "fid-1",
			Statement:  "用户偏好深色模式界面",
			FactKind:   "preference",
			Confidence: 0.93,
			SourceKind: "auto_memory",
		}},
	})
	if err != nil {
		t.Fatalf("WriteBackSessionFacts: %v", err)
	}
	if !res.Created || res.DocID != "doc-wb" || res.Appended != 1 || res.CollectionID != "team-1" {
		t.Fatalf("result = %+v", res)
	}
	if created.RelPath != WriteBackRelPath(time.Now()) && !strings.HasPrefix(created.RelPath, "inbox/writeback-") {
		t.Fatalf("rel_path = %q", created.RelPath)
	}
	if !strings.Contains(created.ContentText, "fact_id: `fid-1`") {
		t.Fatalf("missing provenance: %s", created.ContentText)
	}
	if !strings.Contains(created.ContentText, "session_id: `sess-1`") {
		t.Fatalf("missing session provenance")
	}
}

func TestUsecase_WriteBackSessionFacts_DedupAndSkipLowConf(t *testing.T) {
	m := noOpMockRepo()
	existing := writeBackDocHeader(time.Now()) + "## preference\n\n已有。\n\n- fact_id: `fid-1`\n"
	updated := ""
	m.collListFn = func(_ context.Context, _ string, _, _ int) ([]Collection, int, error) {
		return []Collection{{ID: "team-1", Name: WriteBackCollectionName, VaultBackend: VaultBackendTeam}}, 1, nil
	}
	m.docGetByRelFn = func(_ context.Context, collectionID, rel string) (Document, error) {
		return Document{ID: "doc-wb", CollectionID: collectionID, RelPath: rel, ContentText: existing}, nil
	}
	m.docContentFn = func(_ context.Context, _, content string, _ bool) error {
		updated = content
		return nil
	}
	u := NewUsecaseFromRepo(m)

	res, err := u.WriteBackSessionFacts(context.Background(), WriteBackInput{
		SessionID: "sess-2",
		Facts: []WriteBackFact{
			{FactID: "fid-1", Statement: "用户偏好深色模式界面", FactKind: "preference", Confidence: 0.99},
			{FactID: "fid-2", Statement: "发布必须经过灰度验证门", FactKind: "constraint", Confidence: 0.88},
			{FactID: "fid-3", Statement: "可能要换云厂商", FactKind: "decision", Confidence: 0.5},
		},
	})
	if err != nil {
		t.Fatalf("err %v", err)
	}
	if res.Appended != 1 || res.Created {
		t.Fatalf("result = %+v", res)
	}
	if strings.Count(updated, "fact_id: `fid-1`") != 1 {
		t.Fatalf("fid-1 duplicated: %s", updated)
	}
	if !strings.Contains(updated, "fact_id: `fid-2`") {
		t.Fatalf("fid-2 not appended: %s", updated)
	}
	if strings.Contains(updated, "fid-3") {
		t.Fatalf("low-conf leaked: %s", updated)
	}
}

func TestUsecase_LookupWriteBackHome_NamedInboxWins(t *testing.T) {
	m := noOpMockRepo()
	m.collListFn = func(_ context.Context, ws string, _, _ int) ([]Collection, int, error) {
		return []Collection{
			{ID: "local-1", Name: "个人库", VaultBackend: VaultBackendLocal, Workspace: ws},
			{ID: "team-other", Name: "工程团队", VaultBackend: VaultBackendTeam, Workspace: ws},
			{ID: "inbox", Name: WriteBackCollectionName, VaultBackend: VaultBackendTeam, Workspace: ws},
		}, 3, nil
	}
	u := NewUsecaseFromRepo(m)
	col, found, err := u.LookupWriteBackHome(context.Background(), "ws-1")
	if err != nil {
		t.Fatal(err)
	}
	if !found || col.ID != "inbox" {
		t.Fatalf("got found=%v id=%q", found, col.ID)
	}
}

func TestUsecase_LookupWriteBackHome_FirstTeamWhenNoInbox(t *testing.T) {
	m := noOpMockRepo()
	m.collListFn = func(_ context.Context, ws string, _, _ int) ([]Collection, int, error) {
		return []Collection{
			{ID: "local-1", Name: "个人库", VaultBackend: VaultBackendLocal, Workspace: ws},
			{ID: "team-1", Name: "工程团队", VaultBackend: VaultBackendTeam, Workspace: ws},
		}, 2, nil
	}
	u := NewUsecaseFromRepo(m)
	col, found, err := u.LookupWriteBackHome(context.Background(), "ws-1")
	if err != nil {
		t.Fatal(err)
	}
	if !found || col.ID != "team-1" {
		t.Fatalf("got found=%v id=%q", found, col.ID)
	}
}

func TestUsecase_LookupWriteBackHome_Empty(t *testing.T) {
	m := noOpMockRepo()
	m.collListFn = func(_ context.Context, _ string, _, _ int) ([]Collection, int, error) {
		return []Collection{{ID: "local-1", Name: "个人库", VaultBackend: VaultBackendLocal}}, 1, nil
	}
	u := NewUsecaseFromRepo(m)
	_, found, err := u.LookupWriteBackHome(context.Background(), "ws-1")
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Fatal("expected no write-back home on local-only workspace")
	}
}

func TestUsecase_WriteBackSessionFacts_EmptyAfterGate(t *testing.T) {
	u := NewUsecaseFromRepo(noOpMockRepo())
	res, err := u.WriteBackSessionFacts(context.Background(), WriteBackInput{
		Facts: []WriteBackFact{{Statement: "maybe", FactKind: "fact", Confidence: 0.99}},
	})
	if err != nil || res.Appended != 0 {
		t.Fatalf("want skip, got %+v err=%v", res, err)
	}
}
