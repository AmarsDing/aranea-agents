// Package reliability provides event reliability classification for the event system.
//
// It defines three reliability tiers (Critical, Important, Informational) and a
// generic Classifier[T] that maps event types to reliability guarantees. This
// enables downstream components (Bus delivery policy, WAL, persist handlers) to
// make uniform decisions based on event criticality without maintaining their
// own classification logic.
//
// Usage:
//
//	classifier := reliability.NewClassifier[string]()
//	classifier.Register("tool_result", reliability.Critical)
//	classifier.Register("error", reliability.Critical)
//	classifier.Register("text_delta", reliability.Informational)
//	tier := classifier.Classify("tool_result") // => Critical
package reliability

// Tier defines the reliability guarantee for an event type.
type Tier int

const (
	// Critical events use WBPF (Write-Before-Publish-Fanout).
	// They are persisted to WAL before being published to the Bus.
	// Loss of these events causes observable data corruption.
	Critical Tier = iota

	// Important events use BlockUpTo + async persistence.
	// They are published immediately but persisted asynchronously.
	// They must never be silently dropped.
	Important

	// Informational events are best-effort with no persistence.
	// They may be silently dropped under back-pressure.
	Informational
)

// String returns a human-readable name for the tier.
func (t Tier) String() string {
	switch t {
	case Critical:
		return "critical"
	case Important:
		return "important"
	case Informational:
		return "informational"
	default:
		return "unknown"
	}
}

// RequiresBlockUpTo returns true if the tier must use BlockUpTo delivery
// (i.e., it is Critical or Important — never silently dropped).
func RequiresBlockUpTo(t Tier) bool {
	return t == Critical || t == Important
}

// IsCriticalWBPF returns true if the tier requires WBPF.
func IsCriticalWBPF(t Tier) bool {
	return t == Critical
}

// Classifier maps event types to reliability tiers.
// Type parameter T allows using any comparable type as the event type key
// (e.g., string, int, or a custom enum type).
type Classifier[T comparable] struct {
	classifications map[T]Tier
	fallback        Tier
}

// NewClassifier creates a new Classifier with the given fallback tier
// used for unregistered event types. If fallback is not specified,
// Informational is used as the default.
func NewClassifier[T comparable](fallback ...Tier) *Classifier[T] {
	f := Informational
	if len(fallback) > 0 {
		f = fallback[0]
	}
	return &Classifier[T]{
		classifications: make(map[T]Tier),
		fallback:        f,
	}
}

// Register maps an event type to a reliability tier.
func (c *Classifier[T]) Register(eventType T, tier Tier) {
	c.classifications[eventType] = tier
}

// RegisterBulk maps multiple event types to the same reliability tier.
func (c *Classifier[T]) RegisterBulk(tier Tier, eventTypes ...T) {
	for _, t := range eventTypes {
		c.classifications[t] = tier
	}
}

// Classify returns the reliability tier for an event type.
// Unregistered types fall back to the classifier's default tier.
func (c *Classifier[T]) Classify(eventType T) Tier {
	if tier, ok := c.classifications[eventType]; ok {
		return tier
	}
	return c.fallback
}

// MustClassify returns the reliability tier for an event type.
// Panics if the event type is not registered and no fallback is set.
func (c *Classifier[T]) MustClassify(eventType T) Tier {
	if tier, ok := c.classifications[eventType]; ok {
		return tier
	}
	return c.fallback
}

// IsRegistered returns true if the event type has an explicit classification.
func (c *Classifier[T]) IsRegistered(eventType T) bool {
	_, ok := c.classifications[eventType]
	return ok
}

// Tiers returns all registered event types for a given tier.
func (c *Classifier[T]) Tiers(tier Tier) []T {
	var result []T
	for t, v := range c.classifications {
		if v == tier {
			result = append(result, t)
		}
	}
	return result
}
