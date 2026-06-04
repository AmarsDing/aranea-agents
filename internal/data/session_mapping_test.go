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
		"ID":                        "AgentID",
		"CompressLlmCacheEnabled":   "CompressLLMCacheEnabled",
		"CompressLlmCacheMaxEntries": "CompressLLMCacheMaxEntries",
		"CompressLlmCacheTTLSec":    "CompressLLMCacheTTLSec",
		"ForgetPolicyJSON":          "ForgetConfigJSON",
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
