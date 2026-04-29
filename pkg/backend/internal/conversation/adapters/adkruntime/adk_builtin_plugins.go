package adkruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"google.golang.org/adk/plugin"
	"google.golang.org/adk/plugin/retryandreflect"
)

type builtinPluginDefinition struct {
	Key            string
	Name           string
	Description    string
	Category       string
	RiskLevel      string
	CallbackPoints []string
	Factory        func(map[string]any) (*plugin.Plugin, error)
}

type PluginRuntimeConfig struct {
	Key        string
	ConfigJSON string
}

type ConfiguredPluginSource interface {
	EnabledPluginConfigs(context.Context) ([]PluginRuntimeConfig, error)
}

type BuiltinPluginDefinition struct {
	Key               string
	Name              string
	Description       string
	Category          string
	RiskLevel         string
	CallbackPoints    []string
	DefaultConfigJSON string
	ConfigSchemaJSON  string
	SortOrder         int
}

var builtinPluginRegistry = map[string]builtinPluginDefinition{
	"runtime_audit": {
		Key:         "runtime_audit",
		Name:        "Runtime Audit",
		Description: "记录脱敏后的 Agent、模型、事件和工具执行摘要，用于运行审计。",
		Category:    "observability",
		RiskLevel:   "low",
		CallbackPoints: []string{
			"on_user_message", "before_model", "after_model", "before_tool", "after_tool", "on_tool_error", "on_event",
		},
		Factory: func(_ map[string]any) (*plugin.Plugin, error) { return newRuntimeAuditPlugin() },
	},
	"sensitive_data_mask": {
		Key:            "sensitive_data_mask",
		Name:           "Sensitive Data Mask",
		Description:    "在模型调用前后遮蔽密钥、隐私和敏感数据。",
		Category:       "guard",
		RiskLevel:      "medium",
		CallbackPoints: []string{"on_user_message", "before_model", "after_model"},
		Factory:        func(_ map[string]any) (*plugin.Plugin, error) { return newSensitiveDataMaskPlugin() },
	},
	"confirmation_guard": {
		Key:            "confirmation_guard",
		Name:           "Confirmation Guard",
		Description:    "拦截高风险工具调用，等待后续确认流程接入。",
		Category:       "guard",
		RiskLevel:      "high",
		CallbackPoints: []string{"before_tool"},
		Factory:        func(_ map[string]any) (*plugin.Plugin, error) { return newConfirmationGuardPlugin() },
	},
	"skill_usage_tracker": {
		Key:            "skill_usage_tracker",
		Name:           "Skill Usage Tracker",
		Description:    "统计 Skill 工具的成功、失败、耗时和 Agent 使用摘要。",
		Category:       "tracking",
		RiskLevel:      "low",
		CallbackPoints: []string{"before_tool", "after_tool", "on_tool_error"},
		Factory:        func(_ map[string]any) (*plugin.Plugin, error) { return newSkillUsageTrackerPlugin() },
	},
	"retry_and_reflect": {
		Key:            "retry_and_reflect",
		Name:           "Retry and Reflect",
		Description:    "对可恢复的工具失败触发 ADK 重试与反思流程。",
		Category:       "debug",
		RiskLevel:      "medium",
		CallbackPoints: []string{"on_tool_error"},
		Factory: func(_ map[string]any) (*plugin.Plugin, error) {
			return retryandreflect.New()
		},
	},
	"permission_guard": {
		Key:            "permission_guard",
		Name:           "Permission Guard",
		Description:    "在工具执行前检查权限规则，阻止未授权调用。",
		Category:       "guard",
		RiskLevel:      "high",
		CallbackPoints: []string{"before_tool"},
		Factory:        func(config map[string]any) (*plugin.Plugin, error) { return newPermissionGuardPluginWithConfig(config) },
	},
	"output_policy": {
		Key:            "output_policy",
		Name:           "Output Policy",
		Description:    "拦截危险命令、泄露密钥和违反策略的模型输出。",
		Category:       "policy",
		RiskLevel:      "high",
		CallbackPoints: []string{"after_model"},
		Factory:        func(config map[string]any) (*plugin.Plugin, error) { return newOutputPolicyPluginWithConfig(config) },
	},
	"cost_guard": {
		Key:            "cost_guard",
		Name:           "Cost Guard",
		Description:    "按 token 预算、禁用高价模型和回退路由控制模型成本。",
		Category:       "guard",
		RiskLevel:      "medium",
		CallbackPoints: []string{"before_model"},
		Factory:        func(config map[string]any) (*plugin.Plugin, error) { return newCostGuardPluginWithConfig(config) },
	},
	"model_router": {
		Key:            "model_router",
		Name:           "Model Router",
		Description:    "根据 Agent、任务类型和上下文规模选择模型路由。",
		Category:       "routing",
		RiskLevel:      "medium",
		CallbackPoints: []string{"before_model"},
		Factory:        func(config map[string]any) (*plugin.Plugin, error) { return newModelRouterPluginWithConfig(config) },
	},
}

var builtinPluginAliases = map[string]string{
	"logging":             "runtime_audit",
	"loggingplugin":       "runtime_audit",
	"audit":               "runtime_audit",
	"redaction":           "sensitive_data_mask",
	"mask":                "sensitive_data_mask",
	"confirmation":        "confirmation_guard",
	"confirm":             "confirmation_guard",
	"skill_usage":         "skill_usage_tracker",
	"skill_usage_tracker": "skill_usage_tracker",
	"retry":               "retry_and_reflect",
	"retryandreflect":     "retry_and_reflect",
	"retry_and_reflect":   "retry_and_reflect",
	"cost":                "cost_guard",
	"cost_guard":          "cost_guard",
	"router":              "model_router",
	"model_router":        "model_router",
	"permission":          "permission_guard",
	"permission_guard":    "permission_guard",
	"output":              "output_policy",
	"output_policy":       "output_policy",
}

func BuiltinPluginDefinitions() []BuiltinPluginDefinition {
	keys := make([]string, 0, len(builtinPluginRegistry))
	for key := range builtinPluginRegistry {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]BuiltinPluginDefinition, 0, len(keys))
	for index, key := range keys {
		def := builtinPluginRegistry[key]
		result = append(result, BuiltinPluginDefinition{
			Key:               def.Key,
			Name:              def.Name,
			Description:       def.Description,
			Category:          def.Category,
			RiskLevel:         def.RiskLevel,
			CallbackPoints:    append([]string{}, def.CallbackPoints...),
			DefaultConfigJSON: "{}",
			ConfigSchemaJSON:  "{}",
			SortOrder:         (index + 1) * 10,
		})
	}
	return result
}

func (a *ADKRuntimeAdapter) builtinPlugins(ctx context.Context) ([]*plugin.Plugin, error) {
	if a.pluginSource != nil {
		if source, ok := a.pluginSource.(ConfiguredPluginSource); ok {
			configs, err := source.EnabledPluginConfigs(ctx)
			if err != nil {
				return nil, err
			}
			return builtinPluginsFromConfigs(configs)
		}
		keys, err := a.pluginSource.EnabledPluginKeys(ctx)
		if err != nil {
			return nil, err
		}
		return builtinPluginsFromKeys(keys)
	}
	return builtinPluginsFromEnv()
}

func builtinPluginsFromEnv() ([]*plugin.Plugin, error) {
	raw := strings.TrimSpace(os.Getenv("ADK_RUNNER_PLUGINS"))
	if raw == "" {
		return nil, nil
	}
	keys, err := normalizeBuiltinPluginKeys(raw)
	if err != nil {
		return nil, err
	}
	return builtinPluginsFromKeys(keys)
}

func builtinPluginsFromKeys(keys []string) ([]*plugin.Plugin, error) {
	configs := make([]PluginRuntimeConfig, 0, len(keys))
	for _, key := range keys {
		configs = append(configs, PluginRuntimeConfig{Key: key})
	}
	return builtinPluginsFromConfigs(configs)
}

func builtinPluginsFromConfigs(configs []PluginRuntimeConfig) ([]*plugin.Plugin, error) {
	plugins := make([]*plugin.Plugin, 0, len(configs))
	for _, item := range configs {
		key := item.Key
		if alias, ok := builtinPluginAliases[strings.ToLower(strings.TrimSpace(key))]; ok {
			key = alias
		}
		definition, ok := builtinPluginRegistry[key]
		if !ok {
			return nil, fmt.Errorf("unsupported builtin ADK runner plugin %q", key)
		}
		p, err := definition.Factory(parsePluginConfigJSON(item.ConfigJSON))
		if err != nil {
			return nil, fmt.Errorf("create builtin ADK runner plugin %q: %w", key, err)
		}
		plugins = append(plugins, p)
	}
	return plugins, nil
}

func parsePluginConfigJSON(raw string) map[string]any {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}

func normalizeBuiltinPluginKeys(raw string) ([]string, error) {
	seen := map[string]bool{}
	keys := []string{}
	for _, item := range strings.Split(raw, ",") {
		key := strings.ToLower(strings.TrimSpace(item))
		if key == "" {
			continue
		}
		if alias, ok := builtinPluginAliases[key]; ok {
			key = alias
		}
		if _, ok := builtinPluginRegistry[key]; !ok {
			available := make([]string, 0, len(builtinPluginRegistry))
			for item := range builtinPluginRegistry {
				available = append(available, item)
			}
			sort.Strings(available)
			return nil, fmt.Errorf("unsupported builtin ADK runner plugin %q, available: %s", key, strings.Join(available, ", "))
		}
		if !seen[key] {
			keys = append(keys, key)
			seen[key] = true
		}
	}
	return keys, nil
}
