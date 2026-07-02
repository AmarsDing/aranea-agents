package data

import (
	"reflect"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
)

func TestEntSessionToBizFieldCoverage(t *testing.T) {
	entType := reflect.TypeOf(ent.Session{})
	bizType := reflect.TypeOf(biz.Session{})

	entFields := structFields(entType)
	bizFields := structFields(bizType)

	knownRenames := map[string]string{
		"TotalCostMicroUsd": "TotalCostMicroUSD",
		"McpCallCount":      "MCPCallCount",
	}

	for bizName := range bizFields {
		if _, ok := entFields[bizName]; ok {
			continue
		}
		matched := false
		for entName, mappedBiz := range knownRenames {
			if mappedBiz == bizName {
				if _, ok := entFields[entName]; ok {
					matched = true
				}
				break
			}
		}
		if !matched {
			t.Errorf("biz.Session.%s has no matching field in ent.Session (check if a new field was added to Ent but not mapped)", bizName)
		}
	}
}

func TestEntRuntimeToBizFieldCoverage(t *testing.T) {
	entType := reflect.TypeOf(ent.AgentRuntimeSetting{})
	bizType := reflect.TypeOf(biz.AgentRuntimeSettings{})

	entFields := structFields(entType)
	bizFields := structFields(bizType)

	knownRenames := map[string]string{
		"ID":                         "AgentID",
		"CompressLlmCacheEnabled":    "CompressLLMCacheEnabled",
		"CompressLlmCacheMaxEntries": "CompressLLMCacheMaxEntries",
		"CompressLlmCacheTTLSec":     "CompressLLMCacheTTLSec",
		"ForgetPolicyJSON":           "ForgetConfigJSON",
		"MaxLlmCalls":                "MaxLLMCalls",
	}

	skipBizFields := map[string]bool{
		"CodeExecutorType": true,
	}

	for bizName := range bizFields {
		if skipBizFields[bizName] {
			continue
		}
		if _, ok := entFields[bizName]; ok {
			continue
		}
		matched := false
		for entName, mappedBiz := range knownRenames {
			if mappedBiz == bizName {
				if _, ok := entFields[entName]; ok {
					matched = true
				}
				break
			}
		}
		if !matched {
			t.Errorf("biz.AgentRuntimeSettings.%s has no matching field in ent.AgentRuntimeSetting (check if a new field was added to Ent but not mapped)", bizName)
		}
	}
}

// assertFieldCoverage checks that every exported field in the biz struct has a
// matching field in the ent struct (either by identical name or via knownRenames).
// Fields listed in skipBizFields are excluded (e.g. runtime-only fields not
// persisted, or fields derived via non-trivial transformation).
//
// This catches the most common bug: a new column added to the Ent schema but
// forgotten in the entXxxToBiz conversion function, causing silent data loss.
func assertFieldCoverage(
	t *testing.T,
	entFields, bizFields map[string]bool,
	knownRenames map[string]string, // entFieldName → bizFieldName
	skipBizFields map[string]bool,
	entityName string,
) {
	t.Helper()
	for bizName := range bizFields {
		if skipBizFields[bizName] {
			continue
		}
		if _, ok := entFields[bizName]; ok {
			continue
		}
		matched := false
		for entName, mappedBiz := range knownRenames {
			if mappedBiz == bizName {
				if _, ok := entFields[entName]; ok {
					matched = true
				}
				break
			}
		}
		if !matched {
			t.Errorf("biz.%s.%s has no matching field in ent.%s (check if a new field was added to Ent but not mapped)", entityName, bizName, entityName)
		}
	}
}

func TestEntSessionTurnToBizFieldCoverage(t *testing.T) {
	assertFieldCoverage(t,
		structFields(reflect.TypeOf(ent.SessionTurn{})),
		structFields(reflect.TypeOf(biz.SessionTurn{})),
		map[string]string{
			"TotalCostMicroUsd": "TotalCostMicroUSD",
			"McpCallCount":      "MCPCallCount",
		},
		nil,
		"SessionTurn",
	)
}

func TestEntSessionMetricsToBizFieldCoverage(t *testing.T) {
	assertFieldCoverage(t,
		structFields(reflect.TypeOf(ent.SessionMetrics{})),
		structFields(reflect.TypeOf(biz.SessionMetrics{})),
		map[string]string{
			"ID":                "SessionID", // ent 主键 ID → biz 业务键 SessionID
			"TotalCostMicroUsd": "TotalCostMicroUSD",
			"McpCallCount":      "MCPCallCount",
		},
		nil,
		"SessionMetrics",
	)
}

func TestEntTeamToBizFieldCoverage(t *testing.T) {
	assertFieldCoverage(t,
		structFields(reflect.TypeOf(ent.Team{})),
		structFields(reflect.TypeOf(biz.Team{})),
		map[string]string{
			"AdkAppName":         "ADKAppName",
			"CrossDeptMemberIds": "CrossDeptMemberIDs",
			"DependsOnJSON":      "DependsOn", // JSON string → []string 反序列化
		},
		nil,
		"Team",
	)
}

func TestEntTeamRunToBizFieldCoverage(t *testing.T) {
	assertFieldCoverage(t,
		structFields(reflect.TypeOf(ent.TeamRun{})),
		structFields(reflect.TypeOf(biz.TeamRunRecord{})),
		map[string]string{
			"CostMicroUsd": "CostMicroUSD",
			"DurationMs":   "DurationMS",
		},
		map[string]bool{
			"SpiritSessionID": true, // 运行时元数据，不持久化
		},
		"TeamRun",
	)
}

func TestEntTeamRunStepToBizFieldCoverage(t *testing.T) {
	assertFieldCoverage(t,
		structFields(reflect.TypeOf(ent.TeamRunStep{})),
		structFields(reflect.TypeOf(biz.TeamRunStep{})),
		map[string]string{
			"CostMicroUsd": "CostMicroUSD",
			"DurationMs":   "DurationMS",
		},
		nil,
		"TeamRunStep",
	)
}

func TestEntActivityToBizFieldCoverage(t *testing.T) {
	assertFieldCoverage(t,
		structFields(reflect.TypeOf(ent.Activity{})),
		structFields(reflect.TypeOf(biz.Activity{})),
		nil, // 所有字段名完全一致
		nil,
		"Activity",
	)
}

func structFields(t reflect.Type) map[string]bool {
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	fields := make(map[string]bool)
	for i := 0; i < t.NumField(); i++ {
		name := t.Field(i).Name
		if strings.HasPrefix(name, "__") {
			continue
		}
		fields[name] = true
	}
	return fields
}
