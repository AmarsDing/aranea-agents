package content

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Usecase owns ContentItem lifecycle.
type Usecase struct {
	repo Repo
}

func NewUsecase(repo Repo) *Usecase {
	return &Usecase{repo: repo}
}

// CreateFromIngest inserts or returns the existing row for the same dedupe hash.
func (uc *Usecase) CreateFromIngest(in Item) (Item, bool, error) {
	if uc == nil || uc.repo == nil {
		return Item{}, false, fmt.Errorf("content: repo not configured")
	}
	in.Title = strings.TrimSpace(in.Title)
	in.Body = strings.TrimSpace(in.Body)
	in.SourceURL = strings.TrimSpace(in.SourceURL)
	if in.SourceType == "" {
		in.SourceType = SourceManual
	}
	if in.Status == "" {
		in.Status = StatusCandidate
	}
	if in.DedupeHash == "" {
		in.DedupeHash = DedupeHash(in.SourceURL, in.Title)
	}
	if existing, ok, err := uc.repo.GetByDedupeHash(in.WorkspaceID, in.DedupeHash); err != nil {
		return Item{}, false, err
	} else if ok {
		return existing, true, nil
	}
	now := time.Now().UTC()
	if in.ID == "" {
		in.ID = uuid.NewString()
	}
	in.CreatedAt = now
	in.UpdatedAt = now
	out, err := uc.repo.Create(in)
	return out, false, err
}

func (uc *Usecase) Get(id string) (Item, error) { return uc.repo.Get(id) }

func (uc *Usecase) List(f ListFilter) ([]Item, int, error) {
	if f.Limit <= 0 {
		f.Limit = 50
	}
	return uc.repo.List(f)
}

func (uc *Usecase) SelectForProduction(id string) (Item, error) {
	return uc.move(id, StatusSelected)
}

func (uc *Usecase) MarkDrafting(id string) (Item, error) {
	return uc.move(id, StatusDrafting)
}

func (uc *Usecase) MarkInReview(id string) (Item, error) {
	return uc.move(id, StatusInReview)
}

func (uc *Usecase) MarkApproved(id string) (Item, error) {
	return uc.move(id, StatusApproved)
}

func (uc *Usecase) MarkRejected(id string) (Item, error) {
	return uc.move(id, StatusRejected)
}

func (uc *Usecase) move(id, to string) (Item, error) {
	cur, err := uc.repo.Get(id)
	if err != nil {
		return Item{}, err
	}
	next, err := Transition(cur.Status, to)
	if err != nil {
		return Item{}, err
	}
	cur.Status = next
	cur.UpdatedAt = time.Now().UTC()
	return uc.repo.Update(cur)
}

// DedupeHash builds a stable hash from URL (preferred) or title.
func DedupeHash(sourceURL, title string) string {
	key := strings.TrimSpace(strings.ToLower(sourceURL))
	if key == "" {
		key = strings.TrimSpace(strings.ToLower(title))
	}
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:16])
}
