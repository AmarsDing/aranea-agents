package knowledge

import "testing"

func TestRelationEvidenceSupported(t *testing.T) {
	body := "PostgreSQL 依赖 WAL 保证崩溃恢复。\nRedis 用于短时缓存。"
	for _, evidence := range []string{
		"PostgreSQL 依赖 WAL 保证崩溃恢复",
		"postgresql   依赖 WAL",
	} {
		if !relationEvidenceSupported(body, evidence) {
			t.Errorf("evidence %q should be supported", evidence)
		}
	}
	for _, evidence := range []string{"", "PostgreSQL 使用 Raft 保证一致性"} {
		if relationEvidenceSupported(body, evidence) {
			t.Errorf("hallucinated/empty evidence %q must be rejected", evidence)
		}
	}
}
