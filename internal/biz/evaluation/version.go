package evaluation

import (
	"context"
	"encoding/json"
	"strings"

	"aranea-agents/pkg/apierror"
)

// DatasetVersion is an immutable snapshot of a dataset's cases.
type DatasetVersion struct {
	ID        string
	DatasetID string
	Version   int
	Hash      string
	CaseCount int
	Cases     []Case // populated on Get; omitted on list
	CreatedAt string
}

// VersionStore persists dataset versions.
// Stability:evolving
type VersionStore interface {
	InsertVersion(ctx context.Context, v DatasetVersion) (DatasetVersion, error)
	GetVersion(ctx context.Context, id string) (DatasetVersion, error)
	ListVersions(ctx context.Context, datasetID string, limit int) ([]DatasetVersion, error)
}

// SnapshotDataset writes a new version when the live case hash differs from
// the latest snapshot. Same-hash repeats reuse the latest row.
func (u *Usecase) SnapshotDataset(ctx context.Context, datasetID string) (DatasetVersion, error) {
	datasetID = strings.TrimSpace(datasetID)
	if datasetID == "" {
		return DatasetVersion{}, apierror.BadRequest("EVAL", "dataset_id is required")
	}
	if u.versions == nil {
		return DatasetVersion{}, nil
	}
	cases, err := u.cases.ListCases(ctx, datasetID)
	if err != nil {
		return DatasetVersion{}, err
	}
	hash := HashCases(cases)
	latest, err := u.latestVersion(ctx, datasetID)
	if err != nil {
		return DatasetVersion{}, err
	}
	if latest.ID != "" && latest.Hash == hash {
		return latest, nil
	}
	v := DatasetVersion{
		ID:        newEvalID(),
		DatasetID: datasetID,
		Version:   latest.Version + 1,
		Hash:      hash,
		CaseCount: len(cases),
		Cases:     cases,
	}
	return u.versions.InsertVersion(ctx, v)
}

// ListDatasetVersions returns newest-first snapshots for a dataset.
func (u *Usecase) ListDatasetVersions(ctx context.Context, datasetID string, limit int) ([]DatasetVersion, error) {
	datasetID = strings.TrimSpace(datasetID)
	if datasetID == "" {
		return nil, apierror.BadRequest("EVAL", "dataset_id is required")
	}
	if u.versions == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 50
	}
	return u.versions.ListVersions(ctx, datasetID, limit)
}

// GetDatasetVersion returns one snapshot, including cases.
func (u *Usecase) GetDatasetVersion(ctx context.Context, id string) (DatasetVersion, error) {
	if u.versions == nil || strings.TrimSpace(id) == "" {
		return DatasetVersion{}, apierror.NotFound("EVAL", "dataset version not found")
	}
	return u.versions.GetVersion(ctx, id)
}

func (u *Usecase) latestVersion(ctx context.Context, datasetID string) (DatasetVersion, error) {
	list, err := u.versions.ListVersions(ctx, datasetID, 1)
	if err != nil || len(list) == 0 {
		return DatasetVersion{}, err
	}
	return list[0], nil
}

// MarshalVersionCases encodes snapshot cases for storage.
func MarshalVersionCases(cases []Case) string {
	if len(cases) == 0 {
		return "[]"
	}
	b, err := json.Marshal(cases)
	if err != nil {
		return "[]"
	}
	return string(b)
}

// UnmarshalVersionCases decodes snapshot cases.
func UnmarshalVersionCases(raw string) []Case {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "[]" {
		return nil
	}
	var out []Case
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}
