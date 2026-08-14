package data

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	dataent "aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/skillimportjob"
	"aranea-agents/internal/skill/importer"
	"aranea-agents/pkg/apierror"
)

// skill_import_job_store.go implements importer.SkillImportJobStore via Ent
// (table skill_import_jobs) so ZIP import jobs survive server restarts.
// The importer package defines the interface; the data layer implements it
// (dependency direction: data → importer, no cycle).

// SkillImportJobStore persists skill import jobs to the database.
type SkillImportJobStore struct {
	data *Data
}

var _ importer.SkillImportJobStore = (*SkillImportJobStore)(nil)

// NewSkillImportJobStore constructs the DB-backed import job store.
func NewSkillImportJobStore(d *Data) *SkillImportJobStore {
	return &SkillImportJobStore{data: d}
}

// encodeJSONMap marshals v and re-decodes it into a map suitable for the
// field.JSON(map[string]any) columns ({"items": [...]}).
func encodeJSONMap(v any) (map[string]any, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var items []any
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, err
	}
	return map[string]any{"items": items}, nil
}

func decodeJSONItems[T any](m map[string]any, out *[]T) error {
	raw, err := json.Marshal(m["items"])
	if err != nil {
		return err
	}
	if string(raw) == "null" {
		*out = []T{}
		return nil
	}
	return json.Unmarshal(raw, out)
}

func (s *SkillImportJobStore) Create(ctx context.Context, job biz.SkillImportJob) error {
	candidates, err := encodeJSONMap(job.Candidates)
	if err != nil {
		return entErrToBizErr(err, apierror.DomainSkill)
	}
	groups, err := encodeJSONMap(job.ConflictGroups)
	if err != nil {
		return entErrToBizErr(err, apierror.DomainSkill)
	}
	appliedAt := ""
	if job.Status == "applied" {
		appliedAt = nowRFC3339()
	}
	err = s.data.RW().Write(ctx).SkillImportJob.Create().
		SetID(job.JobID).
		SetStatus(job.Status).
		SetValidationStatus(job.ValidationStatus).
		SetStorageRoot(job.StorageRoot).
		SetMessage(job.Message).
		SetCandidatesJSON(candidates).
		SetConflictGroupsJSON(groups).
		SetTempDir(job.TempDir).
		SetCreatedAt(nowRFC3339()).
		SetAppliedAt(appliedAt).
		Exec(ctx)
	return entErrToBizErr(err, apierror.DomainSkill)
}

// Get returns the job, or (nil, nil) when the job does not exist — the
// engine treats a nil job as a cache miss and falls back to
// ErrImportJobNotFound.
func (s *SkillImportJobStore) Get(ctx context.Context, jobID string) (*biz.SkillImportJob, error) {
	row, err := s.data.RW().Read(ctx).SkillImportJob.Query().
		Where(skillimportjob.IDEQ(strings.TrimSpace(jobID))).
		Only(ctx)
	if err != nil {
		if dataent.IsNotFound(err) {
			return nil, nil
		}
		return nil, entErrToBizErr(err, apierror.DomainSkill)
	}
	job := &biz.SkillImportJob{
		JobID:            row.ID,
		Status:           row.Status,
		ValidationStatus: row.ValidationStatus,
		StorageRoot:      row.StorageRoot,
		Message:          row.Message,
		TempDir:          row.TempDir,
	}
	if err := decodeJSONItems(row.CandidatesJSON, &job.Candidates); err != nil {
		return nil, entErrToBizErr(err, apierror.DomainSkill)
	}
	if err := decodeJSONItems(row.ConflictGroupsJSON, &job.ConflictGroups); err != nil {
		return nil, entErrToBizErr(err, apierror.DomainSkill)
	}
	return job, nil
}

func (s *SkillImportJobStore) UpdateStatus(ctx context.Context, jobID string, status string, message string) error {
	upd := s.data.RW().Write(ctx).SkillImportJob.UpdateOneID(strings.TrimSpace(jobID)).
		SetStatus(status).
		SetMessage(message)
	if status == "applied" {
		upd = upd.SetAppliedAt(nowRFC3339())
	}
	if err := upd.Exec(ctx); err != nil {
		return entErrToBizErr(err, apierror.DomainSkill)
	}
	return nil
}

// CompareAndSwapStatus atomically transitions a job from expectedStatus to
// newStatus. Returns true when the row was updated.
func (s *SkillImportJobStore) CompareAndSwapStatus(ctx context.Context, jobID string, expectedStatus string, newStatus string, message string) (bool, error) {
	upd := s.data.RW().Write(ctx).SkillImportJob.Update().
		Where(
			skillimportjob.IDEQ(strings.TrimSpace(jobID)),
			skillimportjob.StatusEQ(expectedStatus),
		).
		SetStatus(newStatus).
		SetMessage(message)
	if newStatus == "applied" {
		upd = upd.SetAppliedAt(nowRFC3339())
	}
	affected, err := upd.Save(ctx)
	if err != nil {
		return false, entErrToBizErr(err, apierror.DomainSkill)
	}
	return affected > 0, nil
}

func (s *SkillImportJobStore) UpdateCandidates(ctx context.Context, jobID string, candidates []biz.SkillImportCandidate, conflictGroups []biz.SkillConflictGroup) error {
	candidatesJSON, err := encodeJSONMap(candidates)
	if err != nil {
		return entErrToBizErr(err, apierror.DomainSkill)
	}
	groupsJSON, err := encodeJSONMap(conflictGroups)
	if err != nil {
		return entErrToBizErr(err, apierror.DomainSkill)
	}
	if err := s.data.RW().Write(ctx).SkillImportJob.UpdateOneID(strings.TrimSpace(jobID)).
		SetCandidatesJSON(candidatesJSON).
		SetConflictGroupsJSON(groupsJSON).
		Exec(ctx); err != nil {
		return entErrToBizErr(err, apierror.DomainSkill)
	}
	return nil
}

// DeleteOldJobs removes terminal-state jobs older than olderThan (batched,
// ≤100 per call) and returns the temp dirs of deleted rows so the caller can
// clean up candidate files on disk.
func (s *SkillImportJobStore) DeleteOldJobs(ctx context.Context, olderThan time.Duration) (int, []string, error) {
	cutoff := time.Now().UTC().Add(-olderThan).Format(time.RFC3339)
	rows, err := s.data.RW().Read(ctx).SkillImportJob.Query().
		Where(
			skillimportjob.StatusIn("completed", "applied", "failed"),
			skillimportjob.CreatedAtLTE(cutoff),
		).
		Limit(100).
		All(ctx)
	if err != nil {
		return 0, nil, entErrToBizErr(err, apierror.DomainSkill)
	}
	if len(rows) == 0 {
		return 0, nil, nil
	}
	ids := make([]string, 0, len(rows))
	tempDirs := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
		if row.TempDir != "" {
			tempDirs = append(tempDirs, row.TempDir)
		}
	}
	deleted, err := s.data.RW().Write(ctx).SkillImportJob.Delete().
		Where(skillimportjob.IDIn(ids...)).
		Exec(ctx)
	if err != nil {
		return 0, nil, entErrToBizErr(err, apierror.DomainSkill)
	}
	return deleted, tempDirs, nil
}
