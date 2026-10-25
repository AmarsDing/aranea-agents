package knowledge

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/auth"
	"aranea-agents/pkg/safego"
	"github.com/stretchr/testify/require"
)

type teamNoteRepo struct {
	Repo
	mu           sync.Mutex
	col          Collection
	docs         map[string]Document
	replaceErr   error
	replaceCalls int
}

func newTeamNoteFixture() (*Usecase, *teamNoteRepo, context.Context) {
	r := &teamNoteRepo{Repo: noOpMockRepo(), col: Collection{ID: "team", Workspace: "ws-a", VaultBackend: VaultBackendTeam}, docs: map[string]Document{}}
	ctx := auth.NewContext(workspace.WithContext(context.Background(), "ws-a"), &auth.Auth{UserID: 42})
	return NewUsecaseFromRepo(r), r, ctx
}

func (r *teamNoteRepo) GetCollection(context.Context, string) (Collection, error) { return r.col, nil }
func (r *teamNoteRepo) CreateDocument(_ context.Context, d Document) (Document, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, old := range r.docs {
		if old.CollectionID == d.CollectionID && old.RelPath == d.RelPath {
			return Document{}, apierror.Conflict("KNOWLEDGE", "duplicate note path")
		}
	}
	r.docs[d.ID] = d
	return d, nil
}
func (r *teamNoteRepo) GetDocument(_ context.Context, id string) (Document, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	d, ok := r.docs[id]
	if !ok {
		return Document{}, apierror.NotFound("KNOWLEDGE", "document not found")
	}
	return d, nil
}
func (r *teamNoteRepo) ReplaceTeamDocumentContent(_ context.Context, id, baseHash, content string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.replaceCalls++
	if r.replaceErr != nil {
		return false, r.replaceErr
	}
	d, ok := r.docs[id]
	if !ok {
		return false, apierror.NotFound("KNOWLEDGE", "document not found")
	}
	if HashContent(d.ContentText) != baseHash {
		return false, nil
	}
	d.ContentText, d.ContentHash, d.Status, d.ChunkCount = content, HashContent(content), string(DocumentStatePending), 0
	r.docs[id] = d
	return true, nil
}

func TestTeamNote_CreateOwnerAndDuplicatePath(t *testing.T) {
	u, r, ctx := newTeamNoteFixture()
	var replayCalls atomic.Int32
	u.SetWriteBackReplay(func(context.Context, Collection, []PromoteTouchedDoc) error { replayCalls.Add(1); return nil })
	doc, err := u.CreateTeamDocument(ctx, "team", "notes/a.md")
	require.NoError(t, err)
	require.Equal(t, "42", doc.OwnerUserID)
	require.Equal(t, "notes/a.md", doc.RelPath)
	require.Equal(t, HashContent(doc.ContentText), doc.ContentHash)
	require.EqualValues(t, 1, replayCalls.Load())
	_, err = u.CreateTeamDocument(ctx, "team", "notes/a.md")
	require.True(t, apierror.IsCode(err, apierror.CodeConflict))
	require.Len(t, r.docs, 1)
	require.EqualValues(t, 1, replayCalls.Load(), "failed create cannot replay")
}

func TestTeamNote_ConcurrentCreateSamePathHasOneWinner(t *testing.T) {
	u, r, ctx := newTeamNoteFixture()
	var wg sync.WaitGroup
	var wins, conflicts atomic.Int32
	for i := 0; i < 2; i++ {
		wg.Add(1)
		safego.Go(ctx, "team-note-create-test", func() {
			defer wg.Done()
			_, err := u.CreateTeamDocument(ctx, "team", "same.md")
			if err == nil {
				wins.Add(1)
			} else if apierror.IsCode(err, apierror.CodeConflict) {
				conflicts.Add(1)
			}
		})
	}
	wg.Wait()
	require.EqualValues(t, 1, wins.Load())
	require.EqualValues(t, 1, conflicts.Load())
	require.Len(t, r.docs, 1)
}

func TestTeamNote_StaleHashAndStoreFailureNeverReplayOrOverwrite(t *testing.T) {
	for _, failStore := range []bool{false, true} {
		name := "stale_hash"
		if failStore {
			name = "store_failure"
		}
		t.Run(name, func(t *testing.T) {
			u, r, ctx := newTeamNoteFixture()
			doc, err := u.CreateTeamDocument(ctx, "team", "a.md")
			require.NoError(t, err)
			var replays atomic.Int32
			u.SetWriteBackReplay(func(context.Context, Collection, []PromoteTouchedDoc) error { replays.Add(1); return nil })
			base := "stale"
			if failStore {
				base = doc.ContentHash
				r.replaceErr = apierror.Internal("KNOWLEDGE", "injected failure")
			}
			_, conflict, err := u.UpdateTeamDocumentContent(ctx, doc.ID, "new", base)
			if failStore {
				require.Error(t, err)
				require.False(t, conflict)
			} else {
				require.NoError(t, err)
				require.True(t, conflict)
			}
			stored, getErr := r.GetDocument(ctx, doc.ID)
			require.NoError(t, getErr)
			require.Equal(t, doc.ContentText, stored.ContentText)
			require.Equal(t, doc.ContentHash, stored.ContentHash)
			require.Zero(t, replays.Load())
		})
	}
}

func TestTeamNote_ConcurrentCASHasOneWinner(t *testing.T) {
	u, r, ctx := newTeamNoteFixture()
	doc, err := u.CreateTeamDocument(ctx, "team", "a.md")
	require.NoError(t, err)
	var wg sync.WaitGroup
	var wins, conflicts, replays atomic.Int32
	u.SetWriteBackReplay(func(context.Context, Collection, []PromoteTouchedDoc) error { replays.Add(1); return nil })
	for _, content := range []string{"writer A", "writer B"} {
		body := content
		wg.Add(1)
		safego.Go(ctx, "team-note-cas-test", func() {
			defer wg.Done()
			_, conflict, err := u.UpdateTeamDocumentContent(ctx, doc.ID, body, doc.ContentHash)
			if err == nil {
				if conflict {
					conflicts.Add(1)
				} else {
					wins.Add(1)
				}
			}
		})
	}
	wg.Wait()
	require.EqualValues(t, 1, wins.Load())
	require.EqualValues(t, 1, conflicts.Load())
	require.EqualValues(t, 1, replays.Load())
	stored, err := r.GetDocument(ctx, doc.ID)
	require.NoError(t, err)
	require.Contains(t, []string{"writer A", "writer B"}, stored.ContentText)
	require.Equal(t, HashContent(stored.ContentText), stored.ContentHash)
}

func TestTeamNote_ReplayFailureLeavesDurablePendingContent(t *testing.T) {
	u, r, ctx := newTeamNoteFixture()
	doc, err := u.CreateTeamDocument(ctx, "team", "a.md")
	require.NoError(t, err)
	u.SetWriteBackReplay(func(context.Context, Collection, []PromoteTouchedDoc) error {
		return apierror.Internal("KNOWLEDGE", "embed unavailable")
	})
	updated, conflict, err := u.UpdateTeamDocumentContent(ctx, doc.ID, "durable text", doc.ContentHash)
	require.NoError(t, err, "derived index failure cannot discard an accepted edit")
	require.False(t, conflict)
	require.Equal(t, "durable text", updated.ContentText)
	stored, err := r.GetDocument(ctx, doc.ID)
	require.NoError(t, err)
	require.Equal(t, string(DocumentStatePending), stored.Status)
	require.Equal(t, "durable text", stored.ContentText)
	var repaired atomic.Int32
	u.SetWriteBackReplay(func(_ context.Context, _ Collection, touched []PromoteTouchedDoc) error {
		r.mu.Lock()
		defer r.mu.Unlock()
		for _, td := range touched {
			d := r.docs[td.DocID]
			d.Status = string(DocumentStateIndexed)
			d.ChunkCount = 1
			r.docs[td.DocID] = d
			repaired.Add(1)
		}
		return nil
	})
	u.replayTeamDocument(ctx, r.col, stored)
	require.EqualValues(t, 1, repaired.Load())
	stored, err = r.GetDocument(ctx, doc.ID)
	require.NoError(t, err)
	require.Equal(t, string(DocumentStateIndexed), stored.Status)
}

func TestTeamNote_RejectForeignSharedPrivateAndBinary(t *testing.T) {
	for _, tc := range []struct{ name, workspace, visibility, source string }{
		{"foreign", "ws-b", "collection", "a.md"}, {"shared", "", "collection", "a.md"},
		{"private", "ws-a", "private", "a.md"}, {"binary", "ws-a", "collection", "a.pdf"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			u, r, ctx := newTeamNoteFixture()
			r.col.Workspace = tc.workspace
			r.docs["d"] = Document{ID: "d", CollectionID: "team", Source: tc.source, RelPath: tc.source, MimeType: "text/markdown", ContentText: "old", Visibility: tc.visibility, OwnerUserID: "99"}
			_, _, err := u.UpdateTeamDocumentContent(ctx, "d", "new", HashContent("old"))
			require.Error(t, err)
			require.Zero(t, r.replaceCalls, "reject before touching storage")
			require.Equal(t, "old", r.docs["d"].ContentText)
		})
	}
}

func TestTeamNote_EmptyBodyIsAcceptedAndReplayed(t *testing.T) {
	u, r, ctx := newTeamNoteFixture()
	doc, err := u.CreateTeamDocument(ctx, "team", "blank.md")
	require.NoError(t, err)
	var replayed atomic.Int32
	u.SetWriteBackReplay(func(_ context.Context, _ Collection, docs []PromoteTouchedDoc) error {
		r.mu.Lock()
		defer r.mu.Unlock()
		for _, td := range docs {
			d := r.docs[td.DocID]
			d.Status = "indexed"
			d.ChunkCount = 0
			r.docs[td.DocID] = d
			replayed.Add(1)
		}
		return nil
	})
	updated, conflict, err := u.UpdateTeamDocumentContent(ctx, doc.ID, "", doc.ContentHash)
	require.NoError(t, err)
	require.False(t, conflict)
	require.Empty(t, updated.ContentText)
	require.Equal(t, HashContent(""), updated.ContentHash)
	require.Equal(t, "indexed", updated.Status)
	require.Zero(t, updated.ChunkCount)
	require.EqualValues(t, 1, replayed.Load(), "empty accepted body still needs index lifecycle completion")
}
