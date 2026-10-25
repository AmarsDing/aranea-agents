package data

// P0-7 批 D1 钉板（离线，不触 PG）：skill 主键撞号重试的判定与驱动。
// 铁律：
//  1. 仅认 SQLSTATE 23505 + constraint ∈ {skill_pkey, platform_skill_pkey}——
//     skill_key 唯一冲突是真实业务冲突，重试只会原地再撞，必须立即上浮
//     （CodeConflict 翻译在调用方，判定层只看原始错误）；
//  2. 重试上限 3 次——NewSkillID 128bit 随机段下连撞是 ID:生成器故障信号，
//     不允许无限循环；
//  3. 非 pk 错误(含 23503/23502/其他 23505)一律首错即返。

import (
	"errors"
	"fmt"
	"testing"

	"github.com/lib/pq"
)

func skillPKErr(constraint string) error {
	return &pq.Error{Code: pq.ErrorCode("23505"), Constraint: constraint}
}

func TestIsSkillIDConflict(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"skill_pkey 命中", skillPKErr("skill_pkey"), true},
		{"platform_skill_pkey(ent 语义)命中", skillPKErr("platform_skill_pkey"), true},
		{"fmt %w 包装后仍可判定", fmt.Errorf("insert: %w", skillPKErr("skill_pkey")), true},
		{"skill_key 唯一冲突不重试", skillPKErr("skill_skill_key_key"), false},
		{"其他 23505 约束不重试", skillPKErr("skill_version_pkey"), false},
		{"外键 23503 不重试", &pq.Error{Code: pq.ErrorCode("23503"), Constraint: "fk"}, false},
		{"非 pq 错误不重试", errors.New("boom"), false},
		{"nil 不重试", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isSkillIDConflict(tc.err); got != tc.want {
				t.Fatalf("isSkillIDConflict(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestRetrySkillIDConflict(t *testing.T) {
	t.Run("两撞后成功", func(t *testing.T) {
		attempts := 0
		err := retrySkillIDConflict(func() error {
			attempts++
			if attempts < 3 {
				return skillPKErr("skill_pkey")
			}
			return nil
		})
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}
		if attempts != 3 {
			t.Fatalf("attempts = %d, want 3", attempts)
		}
	})
	t.Run("连撞达上限后上浮末错", func(t *testing.T) {
		attempts := 0
		want := skillPKErr("skill_pkey")
		err := retrySkillIDConflict(func() error {
			attempts++
			return want
		})
		if !errors.Is(err, want) {
			t.Fatalf("err = %v, want final conflict error", err)
		}
		if attempts != skillIDConflictMaxAttempts {
			t.Fatalf("attempts = %d, want cap %d", attempts, skillIDConflictMaxAttempts)
		}
	})
	t.Run("skill_key 冲突首错即返不重试", func(t *testing.T) {
		attempts := 0
		want := skillPKErr("skill_skill_key_key")
		err := retrySkillIDConflict(func() error {
			attempts++
			return want
		})
		if !errors.Is(err, want) {
			t.Fatalf("err = %v, want the conflict surfaced", err)
		}
		if attempts != 1 {
			t.Fatalf("attempts = %d, want 1 (no retry on real conflicts)", attempts)
		}
	})
	t.Run("非冲突错误首错即返", func(t *testing.T) {
		attempts := 0
		want := errors.New("dial tcp: refused")
		err := retrySkillIDConflict(func() error {
			attempts++
			return want
		})
		if !errors.Is(err, want) || attempts != 1 {
			t.Fatalf("err = %v attempts = %d, want immediate surface", err, attempts)
		}
	})
}
