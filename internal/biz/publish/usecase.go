package publish

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/review"

	"github.com/google/uuid"
)

// Usecase creates Publish Outbox jobs only after review approval (mode A package).
type Usecase struct {
	repo        Repo
	packageRoot string
	artifacts   biz.ArtifactSaver
}

func NewUsecase(repo Repo, packageRoot string) *Usecase {
	if packageRoot == "" {
		packageRoot = filepath.Join(os.TempDir(), "aranea-publish-packages")
	}
	return &Usecase{repo: repo, packageRoot: packageRoot}
}

// SetArtifactSaver stores package manifests via the normal Artifact path (Package A).
func (uc *Usecase) SetArtifactSaver(s biz.ArtifactSaver) {
	if uc != nil {
		uc.artifacts = s
	}
}

// CreateFromApprovedReview refuses unless review.MustBeApproved passes.
func (uc *Usecase) CreateFromApprovedReview(rt review.Task, platform string) (Job, error) {
	if err := review.MustBeApproved(rt); err != nil {
		return Job{}, err
	}
	if platform == "" {
		platform = "generic"
	}
	key := rt.ID + "|" + platform + "|" + ModePackage
	if existing, ok, err := uc.repo.GetByIdempotency(key); err != nil {
		return Job{}, err
	} else if ok {
		return existing, nil
	}
	now := time.Now().UTC()
	job := Job{
		ID:             uuid.NewString(),
		ReviewTaskID:   rt.ID,
		ContentItemID:  rt.ContentItemID,
		Platform:       platform,
		Mode:           ModePackage,
		Status:         StatusPending,
		IdempotencyKey: key,
		Title:          rt.DraftTitle,
		Body:           rt.DraftBody,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	return uc.repo.Create(job)
}

func (uc *Usecase) ProcessNext(limit int) (int, error) {
	if limit <= 0 {
		limit = 10
	}
	jobs, err := uc.repo.ListByStatus(StatusPending, limit)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, j := range jobs {
		if err := uc.packageOne(j); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

func (uc *Usecase) packageOne(j Job) error {
	next, err := OutboxTransition(j.Status, StatusPackaging)
	if err != nil {
		return err
	}
	j.Status = next
	j.Attempt++
	j.UpdatedAt = time.Now().UTC()
	if _, err := uc.repo.Update(j); err != nil {
		return err
	}
	path, _, err := BuildPackage(uc.packageRoot, j)
	if err != nil {
		j.Status = StatusFailed
		j.LastError = err.Error()
		if j.Attempt >= 3 {
			j.Status = StatusDeadLetter
		} else {
			j.Status = StatusPending // retry
		}
		j.UpdatedAt = time.Now().UTC()
		_, uerr := uc.repo.Update(j)
		if uerr != nil {
			return uerr
		}
		return err
	}
	j.Status = StatusPackaged
	j.PackageDir = filepath.Dir(path)
	j.ArtifactID = j.ID
	j.LastError = ""
	if uc.artifacts != nil {
		manifestBytes, rerr := os.ReadFile(path)
		if rerr == nil {
			sessionID := "mvp-publish-" + j.ID
			if art, aerr := uc.artifacts.Save(context.Background(), sessionID, "manifest.json", "application/json", manifestBytes); aerr == nil {
				j.ArtifactID = art.ID
			}
			if bodyBytes, berr := os.ReadFile(filepath.Join(j.PackageDir, "body.md")); berr == nil {
				_, _ = uc.artifacts.Save(context.Background(), sessionID, "body.md", "text/markdown", bodyBytes)
			}
		}
	}
	j.UpdatedAt = time.Now().UTC()
	_, err = uc.repo.Update(j)
	return err
}

func (uc *Usecase) List(status string, limit int) ([]Job, error) {
	if status == "" {
		status = StatusPackaged
	}
	if limit <= 0 {
		limit = 50
	}
	return uc.repo.ListByStatus(status, limit)
}

func (uc *Usecase) Get(id string) (Job, error) { return uc.repo.Get(id) }

func (uc *Usecase) PackageRoot() string { return uc.packageRoot }

// ValidateTitle is used by package connector Validate.
func ValidateTitle(title string) error {
	if strings.TrimSpace(title) == "" {
		return fmt.Errorf("publish: title required")
	}
	return nil
}
