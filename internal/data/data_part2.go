package data

import (
	"aranea-agents/internal/biz"
	biza2a "aranea-agents/internal/biz/a2a"
	"aranea-agents/pkg/loggateway"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// NewEvalStoresFromData maps the evaluation adapter onto the ISP DTO used by
// Wire / Usecase. Production must consume Stores, not the deprecated Repo.
func NewEvalStoresFromData(d *Data) biz.EvalStores {
	r := NewEvalRepo(d, d.lg)
	if r == nil {
		return biz.EvalStores{}
	}
	return biz.EvalStoresFrom(r)
}

// NewEvalRepoFromData returns the deprecated composed Repo for tests and the
// adapter compile-time check. Production Wire uses NewEvalStoresFromData.
func NewEvalRepoFromData(d *Data) biz.EvalRepo {
	r := NewEvalRepo(d, d.lg)
	if r == nil {
		return nil
	}
	return r
}

func NewA2ARepoFromData(d *Data, lg loggateway.Logger) biz.A2ARepo {
	return NewA2ARepo(d, lg)
}

// NewA2ARemoteAgentCardWriterFromData binds the federation narrow card-writer
// port (biza2a.RemoteAgentCardWriter) to the a2a repo, without widening the
// biz.A2ARepo aggregate interface (T10 design: kept out of RemoteAgentRepo).
func NewA2ARemoteAgentCardWriterFromData(d *Data, lg loggateway.Logger) biza2a.RemoteAgentCardWriter {
	if d == nil || d.RWDB() == nil {
		return nil
	}
	return &a2aRepo{data: d, lg: lg}
}

// generateCatalogID generates a random 24-hex-char ID for agent/team catalog entries.
func generateCatalogID() string {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {

		return fmt.Sprintf("%012x", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}
