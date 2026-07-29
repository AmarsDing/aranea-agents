package a2a

import (
	"aranea-agents/pkg/loggateway"
)

// TrustManager decides whether an organization's trust level permits
// cross-organization calls (design F.5): trusted/neutral allow (still subject
// to policy and quota), untrusted denies. Unknown or empty levels are treated
// as deny (fail-closed) and logged so operators can fix bad data.
type TrustManager struct {
	lg loggateway.Logger
}

// NewTrustManager constructs a TrustManager. A nil logger is tolerated.
func NewTrustManager(lg loggateway.Logger) *TrustManager {
	return &TrustManager{lg: lg}
}

// Check reports whether calls to an org with the given trust level are permitted.
func (m *TrustManager) Check(trustLevel string) bool {
	switch trustLevel {
	case TrustLevelTrusted, TrustLevelNeutral:
		return true
	case TrustLevelUntrusted:
		return false
	default:
		if m != nil && m.lg != nil {
			m.lg.Warn("federation org has unknown trust level; treating as untrusted",
				loggateway.StepID("a2a.fed.trust.check"),
				loggateway.Str("trust_level", trustLevel),
			)
		}
		return false
	}
}
