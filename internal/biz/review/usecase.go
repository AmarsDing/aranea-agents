package review

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// NotifyFn is optional IM/webhook hook after enqueue (noop in tests).
type NotifyFn func(task Task)

// Usecase is the single-reviewer gate. Only Approved may create PublishJob.
type Usecase struct {
	repo   Repo
	notify NotifyFn
}

func NewUsecase(repo Repo) *Usecase {
	return &Usecase{repo: repo}
}

func (uc *Usecase) SetNotify(fn NotifyFn) { uc.notify = fn }

func (uc *Usecase) Enqueue(t Task) (Task, error) {
	if strings.TrimSpace(t.ContentItemID) == "" {
		return Task{}, fmt.Errorf("review: content_item_id required")
	}
	if t.DraftBody == "" && t.DraftTitle == "" {
		return Task{}, fmt.Errorf("review: draft empty")
	}
	t.MachineFlags = SoftScan(t.DraftTitle+"\n"+t.DraftBody, nil)
	t.Status = StatusPending
	now := time.Now().UTC()
	if t.ID == "" {
		t.ID = uuid.NewString()
	}
	t.CreatedAt = now
	t.UpdatedAt = now
	out, err := uc.repo.Create(t)
	if err != nil {
		return Task{}, err
	}
	if uc.notify != nil {
		uc.notify(out)
	}
	return out, nil
}

func (uc *Usecase) ListPending(limit, offset int) ([]Task, int, error) {
	if limit <= 0 {
		limit = 50
	}
	return uc.repo.ListPending(limit, offset)
}

func (uc *Usecase) Get(id string) (Task, error) { return uc.repo.Get(id) }

func (uc *Usecase) Approve(id, reviewerID, note string) (Task, error) {
	return uc.decide(id, StatusApproved, reviewerID, note)
}

func (uc *Usecase) Reject(id, reviewerID, note string) (Task, error) {
	if strings.TrimSpace(note) == "" {
		return Task{}, fmt.Errorf("review: reject requires decision_note")
	}
	return uc.decide(id, StatusRejected, reviewerID, note)
}

func (uc *Usecase) decide(id, to, reviewerID, note string) (Task, error) {
	cur, err := uc.repo.Get(id)
	if err != nil {
		return Task{}, err
	}
	next, err := Transition(cur.Status, to)
	if err != nil {
		return Task{}, err
	}
	now := time.Now().UTC()
	cur.Status = next
	cur.ReviewerID = reviewerID
	cur.DecisionNote = note
	cur.DecidedAt = &now
	cur.UpdatedAt = now
	return uc.repo.Update(cur)
}

// MustBeApproved is the publish gate: returns error unless status=approved.
func MustBeApproved(t Task) error {
	if t.Status != StatusApproved {
		return fmt.Errorf("review: publish blocked until approved (status=%s)", t.Status)
	}
	return nil
}

func (uc *Usecase) GetByGraphExecution(executionID, nodeID string) (Task, bool, error) {
	if uc == nil || uc.repo == nil {
		return Task{}, false, fmt.Errorf("review: repo not configured")
	}
	return uc.repo.GetByGraphExecution(executionID, nodeID)
}

// UpdateLinks persists graph linkage fields without changing decision status.
func (uc *Usecase) UpdateLinks(t Task) (Task, error) {
	if uc == nil || uc.repo == nil {
		return Task{}, fmt.Errorf("review: repo not configured")
	}
	t.UpdatedAt = time.Now().UTC()
	return uc.repo.Update(t)
}
