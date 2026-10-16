package commerce

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// SourcingUsecase is the MVP deep e-commerce path: board only, no orders.
type SourcingUsecase struct {
	repo SupplierRepo
}

func NewSourcingUsecase(repo SupplierRepo) *SourcingUsecase {
	return &SourcingUsecase{repo: repo}
}

func (uc *SourcingUsecase) Create(s SupplierCandidate) (SupplierCandidate, error) {
	s.Name = strings.TrimSpace(s.Name)
	if s.Name == "" {
		return SupplierCandidate{}, fmt.Errorf("commerce: supplier name required")
	}
	if s.Status == "" {
		s.Status = SupplierSuggested
	}
	now := time.Now().UTC()
	if s.ID == "" {
		s.ID = uuid.NewString()
	}
	s.CreatedAt = now
	s.UpdatedAt = now
	return uc.repo.Create(s)
}

func (uc *SourcingUsecase) SetStatus(id, status string) (SupplierCandidate, error) {
	switch status {
	case SupplierSuggested, SupplierContacting, SupplierSampled, SupplierRejected, SupplierApprovedPurchase:
	default:
		return SupplierCandidate{}, fmt.Errorf("commerce: invalid supplier status %q", status)
	}
	cur, err := uc.repo.Get(id)
	if err != nil {
		return SupplierCandidate{}, err
	}
	cur.Status = status
	cur.UpdatedAt = time.Now().UTC()
	// approved_for_purchase is bookkeeping only — never places an order.
	return uc.repo.Update(cur)
}

func (uc *SourcingUsecase) List(limit, offset int) ([]SupplierCandidate, int, error) {
	if limit <= 0 {
		limit = 50
	}
	return uc.repo.List(limit, offset)
}

func (uc *SourcingUsecase) ListByOpportunity(opportunityID string) ([]SupplierCandidate, error) {
	return uc.repo.ListByOpportunity(opportunityID)
}
