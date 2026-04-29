package repository

import (
	"arenea/backend/internal/capability/adapters/sqlite"
	"arenea/backend/internal/domain"
)

func (r *SQLiteRepository) skillRepo() *sqlite.SkillRepository {
	return sqlite.NewSkillRepository(r.db)
}

func (r *SQLiteRepository) SearchSkills(query domain.SkillListQuery) (domain.SkillListResult, error) {
	return r.skillRepo().SearchSkills(query)
}

func (r *SQLiteRepository) GetSkillByID(id string) (domain.Skill, error) {
	return r.skillRepo().GetSkillByID(id)
}

func (r *SQLiteRepository) UpdateSkillEnabled(id string, enabled bool) (domain.Skill, error) {
	return r.skillRepo().UpdateSkillEnabled(id, enabled)
}

func (r *SQLiteRepository) DuplicateSkill(id string) (domain.Skill, error) {
	return r.skillRepo().DuplicateSkill(id)
}

func (r *SQLiteRepository) DeleteSkill(id string) error {
	return r.skillRepo().DeleteSkill(id)
}

func (r *SQLiteRepository) SearchSkillInvocations(query domain.SkillRunQuery) (domain.SkillRunResult, error) {
	return r.skillRepo().SearchSkillInvocations(query)
}

func (r *SQLiteRepository) ListSkillSimilaritySources() ([]domain.SkillSimilaritySource, error) {
	return r.skillRepo().ListSkillSimilaritySources()
}

func (r *SQLiteRepository) CreateSkillWithVersion(input domain.SkillCreateInput) (domain.Skill, error) {
	return r.skillRepo().CreateSkillWithVersion(input)
}

func (r *SQLiteRepository) GetSkillStorageDir(id string) (string, error) {
	return r.skillRepo().GetSkillStorageDir(id)
}
