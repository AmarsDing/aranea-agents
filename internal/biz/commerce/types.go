package commerce

import "time"

const (
	AudienceNovaTradeIntel   = "novatrade_intel"
	AudienceCommerceSourcing = "commerce_sourcing"
	AudienceBoth             = "both"

	OppOpen         = "open"
	OppAcknowledged = "acknowledged"
	OppDismissed    = "dismissed"
	OppConverted    = "converted"

	SupplierSuggested          = "suggested"
	SupplierContacting         = "contacting"
	SupplierSampled            = "sampled"
	SupplierRejected           = "rejected"
	SupplierApprovedPurchase   = "approved_for_purchase" // bookkeeping only — NEVER places an order
)

// Opportunity is a 50/50 NovaTrade intel vs commerce sourcing card.
type Opportunity struct {
	ID        string
	Title     string
	Summary   string
	Entities  []string
	Signals   []string // content_item ids
	Audience  string
	Score     float64
	Status    string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// SupplierCandidate is the sourcing-board deep path (no real orders).
type SupplierCandidate struct {
	ID            string
	OpportunityID string
	Name          string
	URL           string
	PriceRange    string
	Notes         string
	Status        string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type OpportunityRepo interface {
	Create(o Opportunity) (Opportunity, error)
	Get(id string) (Opportunity, error)
	Update(o Opportunity) (Opportunity, error)
	List(audience, status string, limit, offset int) ([]Opportunity, int, error)
}

type SupplierRepo interface {
	Create(s SupplierCandidate) (SupplierCandidate, error)
	Get(id string) (SupplierCandidate, error)
	Update(s SupplierCandidate) (SupplierCandidate, error)
	ListByOpportunity(opportunityID string) ([]SupplierCandidate, error)
	List(limit, offset int) ([]SupplierCandidate, int, error)
}
