package data

import (
	"context"
	"encoding/json"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/skillimportjob"
	"aranea-agents/pkg/loggateway"
)

// SkillImportJobRepo persists skill import job state to the database,
// replacing the in-memory map so that jobs survive server restarts.
type SkillImportJobRepo struct {
	data *Data
	lg   loggateway.Logger
}

// NewSkillImportJobRepo creates a new SkillImportJobRepo.
func NewSkillImportJobRepo(data *Data, lg loggateway.Logger) *SkillImportJobRepo {
	return &SkillImportJobRepo{data: data, lg: lg}
}

// Create persists a new import job.
func (r *SkillImportJobRepo) Create(ctx context.Context, job biz.SkillImportJob) error {
	candidatesJSON, err := json.Marshal(job.Candidates)
	if err != nil {
		return entErrToBizErr(err, apierror.DomainSkill)
	}
	conflictGroupsJSON, err := json.Marshal(job.ConflictGroups)
	if err != nil {
		return entErrToBizErr(err, apierror.DomainSkill)
	}
	builder := r.data.RW().Write(ctx).SkillImportJob.Create().
		SetID(job.JobID).
		SetStatus(job.Status).
		SetValidationStatus(job.ValidationStatus).
		SetStorageRoot(job.StorageRoot).
		SetMessage(job.Message).
		SetCandidatesJSON(rawToMap(candidatesJSON)).
		SetConflictGroupsJSON(rawToMap(conflictGroupsJSON)).
		SetCreatedAt(time.Now().UTC().Format(time.RFC3339))
	if job.TempDir != "" {
		builder.SetTempDir(job.TempDir)
	}
	_, err = builder.Save(ctx)
	return err
}

// Get retrieves an import job by ID.
func (r *SkillImportJobRepo) Get(ctx context.Context, jobID string) (*biz.SkillImportJob, error) {
	row, err := r.data.RW().Read(ctx).SkillImportJob.Get(ctx, jobID)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return mapEntSkillImportJob(row), nil
}

// UpdateStatus updates the status and message of an import job.
func (r *SkillImportJobRepo) UpdateStatus(ctx context.Context, jobID string, status string, message string) error {
	builder := r.data.RW().Write(ctx).SkillImportJob.UpdateOneID(jobID).
		SetStatus(status)
	if message != "" {
		builder.SetMessage(message)
	}
	if status == "applied" {
		builder.SetAppliedAt(time.Now().UTC().Format(time.RFC3339))
	}
	_, err := builder.Save(ctx)
	return err
}

// UpdateCandidates persists updated candidates and conflict groups for a job.
func (r *SkillImportJobRepo) UpdateCandidates(ctx context.Context, jobID string, candidates []biz.SkillImportCandidate, conflictGroups []biz.SkillConflictGroup) error {
	candidatesJSON, err := json.Marshal(candidates)
	if err != nil {
		return entErrToBizErr(err, apierror.DomainSkill)
	}
	conflictGroupsJSON, err := json.Marshal(conflictGroups)
	if err != nil {
		return entErrToBizErr(err, apierror.DomainSkill)
	}
	_, err = r.data.RW().Write(ctx).SkillImportJob.UpdateOneID(jobID).
		SetCandidatesJSON(rawToMap(candidatesJSON)).
		SetConflictGroupsJSON(rawToMap(conflictGroupsJSON)).
		Save(ctx)
	return err
}

// DeleteOldJobs removes completed/applied jobs older than the given duration.
func (r *SkillImportJobRepo) DeleteOldJobs(ctx context.Context, olderThan time.Duration) (int, error) {
	cutoff := time.Now().UTC().Add(-olderThan).Format(time.RFC3339)
	ids, err := r.data.RW().Read(ctx).SkillImportJob.Query().
		Where(
			skillimportjob.StatusIn("completed", "applied", "failed"),
			skillimportjob.CreatedAtLTE(cutoff),
		).
		Limit(100).
		IDs(ctx)
	if err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, nil
	}
	deleted, err := r.data.RW().Write(ctx).SkillImportJob.Delete().
		Where(skillimportjob.IDIn(ids...)).
		Exec(ctx)
	if err != nil {
		return 0, err
	}
	r.lg.Info("cleaned up old import jobs",
		loggateway.StepID("skill_import.cleanup"),
		loggateway.Int("deleted", deleted))
	return deleted, nil
}

// rawToMap converts a JSON byte slice to map[string]any for Ent JSON fields.
func rawToMap(data []byte) map[string]any {
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return map[string]any{"raw": string(data)}
	}
	return m
}

// mapEntSkillImportJob maps an Ent SkillImportJob row to a biz type.
func mapEntSkillImportJob(row *ent.SkillImportJob) *biz.SkillImportJob {
	job := &biz.SkillImportJob{
		JobID:            row.ID,
		Status:           row.Status,
		ValidationStatus: row.ValidationStatus,
		StorageRoot:      row.StorageRoot,
		Message:          row.Message,
		TempDir:          row.TempDir,
		Candidates:       []biz.SkillImportCandidate{},
		ConflictGroups:   []biz.SkillConflictGroup{},
	}
	if row.CandidatesJSON != nil {
		if data, err := json.Marshal(row.CandidatesJSON); err == nil {
			_ = json.Unmarshal(data, &job.Candidates)
		}
	}
	if row.ConflictGroupsJSON != nil {
		if data, err := json.Marshal(row.ConflictGroupsJSON); err == nil {
			_ = json.Unmarshal(data, &job.ConflictGroups)
		}
	}
	return job
}
