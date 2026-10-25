package service

import (
	"errors"
	"testing"

	"aranea-agents/pkg/loggateway"
)

func getenvFrom(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

// 批 C Q3（2026-10-22）conf 挂载优先级钉死：override>0 时 conf 胜出（即使
// env 同时声明），override<=0 纯 env 回落；override 为 int32 类型化输入
// ParseError 恒 false；ENFORCE 仍仅经 env（与 conf 副本数可自由组合）。
func TestResolveReplicaSafetyWithOverride_Matrix(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		replicas  string // env ARANEA_DECLARED_REPLICAS
		enforce   string // env ARANEA_REPLICA_ENFORCE
		override  int32  // conf server.declared_replicas
		wantMode  string
		wantN     int
		wantParse bool
	}{
		{"override_zero_env_unset", "", "", 0, CoordinationModeSingle, 0, false},
		{"override_zero_env_two", "2", "", 0, CoordinationModeMultiWarn, 2, false},
		{"override_wins_over_env", "3", "", 2, CoordinationModeMultiWarn, 2, false},
		{"override_one_suppresses_env_multi", "5", "", 1, CoordinationModeSingle, 1, false},
		{"override_multi_env_unset", "", "", 4, CoordinationModeMultiWarn, 4, false},
		{"override_multi_with_env_enforce", "", "1", 3, CoordinationModeMultiEnforced, 3, false},
		{"override_negative_falls_back_env", "2", "", -1, CoordinationModeMultiWarn, 2, false},
		{"override_negative_env_bad_parse", "abc", "", -3, CoordinationModeSingle, 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			env := map[string]string{}
			if tc.replicas != "" {
				env[EnvDeclaredReplicas] = tc.replicas
			}
			if tc.enforce != "" {
				env[EnvReplicaEnforce] = tc.enforce
			}
			st := ResolveReplicaSafetyWithOverride(getenvFrom(env), tc.override)
			if st.Mode != tc.wantMode {
				t.Fatalf("Mode=%q, want %q", st.Mode, tc.wantMode)
			}
			if st.DeclaredReplicas != tc.wantN {
				t.Fatalf("DeclaredReplicas=%d, want %d", st.DeclaredReplicas, tc.wantN)
			}
			if st.ParseError != tc.wantParse {
				t.Fatalf("ParseError=%v, want %v", st.ParseError, tc.wantParse)
			}
		})
	}
}

// 批 C Q3：WithOverride 启动自检与 env 入口共用执行体——override>1 同样落
// multi_warn 快照（healthz 一致）且 fail-fast 语义不变。
func TestRunReplicaSafetyStartupCheckWithOverride_MultiWarnSnapshot(t *testing.T) {
	st, err := RunReplicaSafetyStartupCheckWithOverride(loggateway.NewNoop(), getenvFrom(map[string]string{}), 2)
	if err != nil {
		t.Fatalf("multi_warn must not return error: %v", err)
	}
	if st.Mode != CoordinationModeMultiWarn || st.DeclaredReplicas != 2 {
		t.Fatalf("status = %+v, want multi_warn/2", st)
	}
	if cur := CurrentReplicaSafety(); cur.Mode != CoordinationModeMultiWarn || cur.DeclaredReplicas != 2 {
		t.Fatalf("snapshot = %+v, want multi_warn/2", cur)
	}
	defer func() { _, _ = RunReplicaSafetyStartupCheck(loggateway.NewNoop(), getenvFrom(map[string]string{})) }()
}

// 规则钉死：未声明(空/0) 或 1 → single；>1 → multi_warn；>1+enforce → multi_enforced；
// 坏输入（非数字/负数）必拒 → ParseError=true 且按未声明（single）处理。
func TestResolveReplicaSafety_Matrix(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		replicas  string
		enforce   string
		wantMode  string
		wantN     int
		wantParse bool
	}{
		{"empty", "", "", CoordinationModeSingle, 0, false},
		{"zero", "0", "", CoordinationModeSingle, 0, false},
		{"one", "1", "", CoordinationModeSingle, 1, false},
		{"one_with_whitespace", " 1 ", "", CoordinationModeSingle, 1, false},
		{"two", "2", "", CoordinationModeMultiWarn, 2, false},
		{"many", "17", "", CoordinationModeMultiWarn, 17, false},
		{"single_with_enforce_off", "1", "1", CoordinationModeSingle, 1, false},
		{"multi_enforce_on_1", "3", "1", CoordinationModeMultiEnforced, 3, false},
		{"multi_enforce_on_true", "3", "True", CoordinationModeMultiEnforced, 3, false},
		{"multi_enforce_off_0", "3", "0", CoordinationModeMultiWarn, 3, false},
		{"multi_enforce_off_garbage", "3", "enabled", CoordinationModeMultiWarn, 3, false},
		{"bad_input_alpha", "abc", "", CoordinationModeSingle, 0, true},
		{"bad_input_negative", "-2", "", CoordinationModeSingle, 0, true},
		{"bad_input_float", "1.5", "", CoordinationModeSingle, 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			env := map[string]string{}
			if tc.replicas != "" {
				env[EnvDeclaredReplicas] = tc.replicas
			}
			if tc.enforce != "" {
				env[EnvReplicaEnforce] = tc.enforce
			}
			st := ResolveReplicaSafety(getenvFrom(env))
			if st.Mode != tc.wantMode {
				t.Fatalf("Mode=%q, want %q", st.Mode, tc.wantMode)
			}
			if st.DeclaredReplicas != tc.wantN {
				t.Fatalf("DeclaredReplicas=%d, want %d", st.DeclaredReplicas, tc.wantN)
			}
			if st.ParseError != tc.wantParse {
				t.Fatalf("ParseError=%v, want %v", st.ParseError, tc.wantParse)
			}
		})
	}
}

// 零行为变化钉死：未声明/声明 1 时自检令牌内不得发出任何 Warn/Error 且返回 nil error。
func TestRunReplicaSafetyStartupCheck_SingleIsSilent(t *testing.T) {
	t.Parallel()
	for _, replicas := range []string{"", "0", "1"} {
		st, err := RunReplicaSafetyStartupCheck(loggateway.NewNoop(), getenvFrom(map[string]string{
			EnvDeclaredReplicas: replicas,
			EnvReplicaEnforce:   "1", // enforce 在 single 下不应触发
		}))
		if err != nil {
			t.Fatalf("replicas=%q: unexpected error %v", replicas, err)
		}
		if st.Mode != CoordinationModeSingle {
			t.Fatalf("replicas=%q: Mode=%q, want single", replicas, st.Mode)
		}
	}
}

// multi_warn：返回 nil error（降级方式运行），status 逐项可见。
func TestRunReplicaSafetyStartupCheck_MultiWarn(t *testing.T) {
	t.Parallel()
	st, err := RunReplicaSafetyStartupCheck(loggateway.NewNoop(), getenvFrom(map[string]string{
		EnvDeclaredReplicas: "3",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if st.Mode != CoordinationModeMultiWarn || st.DeclaredReplicas != 3 {
		t.Fatalf("st=%+v", st)
	}
}

// fail-fast：>1 + ENFORCE 确定性返回 ErrMultiReplicaEnforced。
func TestRunReplicaSafetyStartupCheck_EnforceFailsFast(t *testing.T) {
	t.Parallel()
	st, err := RunReplicaSafetyStartupCheck(loggateway.NewNoop(), getenvFrom(map[string]string{
		EnvDeclaredReplicas: "2",
		EnvReplicaEnforce:   "true",
	}))
	if !errors.Is(err, ErrMultiReplicaEnforced) {
		t.Fatalf("err=%v, want ErrMultiReplicaEnforced", err)
	}
	if st.Mode != CoordinationModeMultiEnforced {
		t.Fatalf("Mode=%q, want multi_enforced", st.Mode)
	}
}

// 对抗评审 #15：override 路径（conf server.declared_replicas>1）+ env ENFORCE
// 同样 must fail-fast——Q3 挂载后 override 是生产主配置面，fail-fast 语义不可
// 只测 env 分支。
func TestRunReplicaSafetyStartupCheckWithOverride_EnforceFailsFast(t *testing.T) {
	st, err := RunReplicaSafetyStartupCheckWithOverride(loggateway.NewNoop(), getenvFrom(map[string]string{
		EnvReplicaEnforce: "1",
	}), 3)
	if !errors.Is(err, ErrMultiReplicaEnforced) {
		t.Fatalf("err=%v, want ErrMultiReplicaEnforced", err)
	}
	if st.Mode != CoordinationModeMultiEnforced || st.DeclaredReplicas != 3 {
		t.Fatalf("st = %+v, want multi_enforced/3", st)
	}
	// 快照复位，防污染同包后续用例（本用例不可 t.Parallel——包内自检快照共享）。
	defer func() { _, _ = RunReplicaSafetyStartupCheck(loggateway.NewNoop(), getenvFrom(map[string]string{})) }()
}

// healthz 快照：自检运行前后 CurrentReplicaSafety 语义一致切换。
func TestCurrentReplicaSafety_SnapshotAfterCheck(t *testing.T) {
	st := CurrentReplicaSafety() // 冷态：按实际环境解析（测试环境未设变量 → single）
	if st.Mode != CoordinationModeSingle {
		t.Fatalf("cold Mode=%q, want single", st.Mode)
	}
	_, _ = RunReplicaSafetyStartupCheck(loggateway.NewNoop(), getenvFrom(map[string]string{
		EnvDeclaredReplicas: "5",
	}))
	got := CurrentReplicaSafety()
	if got.Mode != CoordinationModeMultiWarn || got.DeclaredReplicas != 5 {
		t.Fatalf("after check got=%+v", got)
	}
	// 复位，避免污染同包其他测试（本测试不并行）。
	_, _ = RunReplicaSafetyStartupCheck(loggateway.NewNoop(), getenvFrom(map[string]string{}))
}

// 单副本清单钉死 9 项且键唯一、锚点非空（DOC-SYNC-6：锚点契约同步 replica-safety.md）。
func TestSingleReplicaFeatures_Inventory(t *testing.T) {
	t.Parallel()
	fs := singleReplicaFeatures()
	if len(fs) != 9 {
		t.Fatalf("len=%d, want 9", len(fs))
	}
	seen := make(map[string]bool, len(fs))
	for _, f := range fs {
		if f.Key == "" || f.Description == "" || f.Anchor == "" {
			t.Fatalf("empty field in %+v", f)
		}
		if seen[f.Key] {
			t.Fatalf("duplicate key %q", f.Key)
		}
		seen[f.Key] = true
	}
}
