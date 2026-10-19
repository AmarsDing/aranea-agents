package biz

import (
	"reflect"
	"testing"
)

// inheritAPIUncoveredSettings 继承列清单（维护规则 F11：新增
// agent_runtime_settings 列若不走 proto/narrow-RPC，必须加进函数并同步此表）。
var inheritAPIUncoveredColumns = []string{
	// Memory: L0/L4 knobs and memory-butler state not exposed via the API.
	"L0CompressMinGapSec", "L0CompressMaxTriggerTokens",
	"L4DecayIntervalHours", "L4DecayOverridesJSON", "ForgetConfigJSON",
	// Tools: narrow-RPC-owned (execution timeout) and analysis-owned columns.
	"ToolsExecutionTimeoutSec", "ToolWeightJSON", "MaxLLMCalls",
	"MaxToolIterations", "EnableTokenTailoring", "TokenTailoringStrategy",
	"TokenTailoringSafetyMargin",
	// Governance thresholds (loop guard).
	"LoopGuardToolLoadMax", "LoopGuardWallSoftSec", "LoopGuardWallHardSec",
	// Skills/intent fast path.
	"IntentSkipEnabled",
	// Evolution: dream-cycle rollback snapshots.
	"DreamSnapshotJSON",
	// Context assembly: budgets and gates without proto coverage.
	"CompressionBufferAdaptive", "AssemblyBudgetSoftTokens",
	"AssemblyBudgetHardTokens", "ClarificationEnabled", "ReplyReminderEnabled",
}

// P0 root-fix (2026-10-18) guard: every API-uncovered column must be inherited
// from the persisted row before the fat UpdateAgent upsert. Distinct per-column
// sentinels catch both omitted copies and wrong-column copies (dst.A = cur.B).
func TestInheritAPIUncoveredSettings_CopiesAllColumns(t *testing.T) {
	if len(inheritAPIUncoveredColumns) != 22 {
		t.Fatalf("expected 22 API-uncovered columns, listed %d", len(inheritAPIUncoveredColumns))
	}
	cur := &AgentRuntimeSettings{}
	dst := &AgentRuntimeSettings{}
	cv := reflect.ValueOf(cur).Elem()

	for i, name := range inheritAPIUncoveredColumns {
		f := cv.FieldByName(name)
		if !f.IsValid() || !f.CanSet() {
			t.Fatalf("field %s missing on AgentRuntimeSettings", name)
		}
		switch f.Kind() {
		case reflect.Int, reflect.Int32, reflect.Int64:
			f.SetInt(int64(i+1) * 7)
		case reflect.Float64:
			f.SetFloat(float64(i+1) / 8.0)
		case reflect.String:
			f.SetString(name + "-sentinel")
		case reflect.Bool:
			f.SetBool(true)
		default:
			t.Fatalf("unhandled kind %s for field %s", f.Kind(), name)
		}
	}

	inheritAPIUncoveredSettings(dst, cur)

	dv := reflect.ValueOf(dst).Elem()
	copiedSet := make(map[string]bool, len(inheritAPIUncoveredColumns))
	for _, name := range inheritAPIUncoveredColumns {
		copiedSet[name] = true
		if got, want := dv.FieldByName(name).Interface(), cv.FieldByName(name).Interface(); got != want {
			t.Errorf("field %s: got %v, want %v", name, got, want)
		}
	}
	// Reverse guard: nothing outside the 22-column set may be inherited — a
	// newly added inherit row must be registered in inheritAPIUncoveredColumns.
	zero := reflect.New(dv.Type()).Elem()
	for i := 0; i < dv.NumField(); i++ {
		name := dv.Type().Field(i).Name
		if copiedSet[name] {
			continue
		}
		if got, want := dv.Field(i).Interface(), zero.Field(i).Interface(); !reflect.DeepEqual(got, want) {
			t.Errorf("field %s unexpectedly inherited: got %v (register it in the test list if intentional)", name, got)
		}
	}
}

// Bools share a type, so a wrong-column copy among bool fields could pass the
// value-equality check above when both happen to match. Pin each bool column
// individually: only that column set in cur must produce exactly that column
// set in dst.
func TestInheritAPIUncoveredSettings_BoolColumnsIndividually(t *testing.T) {
	boolCols := []string{
		"EnableTokenTailoring", "IntentSkipEnabled",
		"CompressionBufferAdaptive", "ClarificationEnabled", "ReplyReminderEnabled",
	}
	for _, name := range boolCols {
		t.Run(name, func(t *testing.T) {
			cur := &AgentRuntimeSettings{}
			dst := &AgentRuntimeSettings{}
			reflect.ValueOf(cur).Elem().FieldByName(name).SetBool(true)
			inheritAPIUncoveredSettings(dst, cur)
			dv := reflect.ValueOf(dst).Elem()
			for _, other := range boolCols {
				got := dv.FieldByName(other).Bool()
				want := other == name
				if got != want {
					t.Errorf("field %s = %v, want %v (only %s set in cur)", other, got, want, name)
				}
			}
		})
	}
}

func TestInheritAPIUncoveredSettings_NilSafe(t *testing.T) {
	dst := &AgentRuntimeSettings{}
	inheritAPIUncoveredSettings(nil, &AgentRuntimeSettings{MaxLLMCalls: 3}) // must not panic
	inheritAPIUncoveredSettings(dst, nil)                                   // must not panic
	if *dst != (AgentRuntimeSettings{}) {
		t.Fatalf("nil cur must leave dst untouched, got %+v", dst)
	}
}
