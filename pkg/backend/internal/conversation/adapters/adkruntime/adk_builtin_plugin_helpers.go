package adkruntime

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/model"
	"google.golang.org/genai"
)

func recordSkillUsage(toolName string, args map[string]any, success bool) {
	key := toolInvocationKey(toolName, args)
	started := time.Now()
	skillUsageStats.Lock()
	if value, ok := skillUsageStats.started[key]; ok {
		started = value
		delete(skillUsageStats.started, key)
	}
	record := skillUsageStats.records[toolName]
	if record == nil {
		record = &skillUsageRecord{LastTool: toolName}
		skillUsageStats.records[toolName] = record
	}
	record.InvokeCount++
	if success {
		record.Success++
		record.LastStatus = "success"
	} else {
		record.Failure++
		record.LastStatus = "failure"
	}
	record.DurationMS += int(time.Since(started).Milliseconds())
	record.LastAt = time.Now()
	snapshot := *record
	skillUsageStats.Unlock()
	log.Printf("adk plugin skill_usage_tracker tool=%q status=%s invoke_count=%d success=%d failure=%d duration_ms=%d", toolName, snapshot.LastStatus, snapshot.InvokeCount, snapshot.Success, snapshot.Failure, int(time.Since(started).Milliseconds()))
}

type costGuardConfig struct {
	MaxPromptTokens    int
	BlockedModels      map[string]bool
	FallbackModel      string
	DefaultModel       string
	BlockPremiumModels bool
}

func costGuardConfigFromEnv() costGuardConfig {
	return costGuardConfigFromConfig(nil)
}

func costGuardConfigFromConfig(config map[string]any) costGuardConfig {
	return costGuardConfig{
		MaxPromptTokens:    configInt(config, "max_prompt_tokens", "ADK_COST_MAX_PROMPT_TOKENS", 0),
		BlockedModels:      configSet(config, "blocked_models", "ADK_COST_BLOCKED_MODELS"),
		FallbackModel:      configString(config, "fallback_model", "ADK_COST_FALLBACK_MODEL"),
		DefaultModel:       configString(config, "default_model", "ADK_COST_DEFAULT_MODEL"),
		BlockPremiumModels: configBool(config, "block_premium_models", "ADK_COST_BLOCK_PREMIUM_MODELS", true),
	}
}

type modelRouterConfig struct {
	DefaultModel         string
	CodeModel            string
	LongContextModel     string
	LongContextThreshold int
	AgentModels          map[string]string
}

func modelRouterConfigFromEnv() modelRouterConfig {
	return modelRouterConfigFromConfig(nil)
}

func modelRouterConfigFromConfig(config map[string]any) modelRouterConfig {
	return modelRouterConfig{
		DefaultModel:         configString(config, "default_model", "ADK_ROUTER_DEFAULT_MODEL"),
		CodeModel:            configString(config, "code_model", "ADK_ROUTER_CODE_MODEL"),
		LongContextModel:     configString(config, "long_context_model", "ADK_ROUTER_LONG_CONTEXT_MODEL"),
		LongContextThreshold: configInt(config, "long_context_tokens", "ADK_ROUTER_LONG_CONTEXT_TOKENS", 8000),
		AgentModels:          configMap(config, "agent_models", "ADK_ROUTER_AGENT_MODELS"),
	}
}

func blockedModelResponse(message string) *model.LLMResponse {
	return &model.LLMResponse{
		Content:      genai.NewContentFromText(message, genai.RoleModel),
		ErrorCode:    "MODEL_POLICY_BLOCKED",
		ErrorMessage: message,
		TurnComplete: true,
	}
}

func isPremiumModel(name string) bool {
	modelName := strings.ToLower(strings.TrimSpace(name))
	if modelName == "" {
		return false
	}
	patterns := []string{"opus", "gpt-4.5", "gpt-4o", "gpt-5", "o1", "o3", "pro", "ultra"}
	for _, pattern := range patterns {
		if strings.Contains(modelName, pattern) {
			return true
		}
	}
	return false
}

func looksLikeCodeTask(text string) bool {
	lower := strings.ToLower(text)
	keywords := []string{"code", "golang", "typescript", "javascript", "python", "sql", "debug", "stack trace", "function", "class", "编程", "代码", "报错", "实现"}
	for _, keyword := range keywords {
		if strings.Contains(lower, keyword) {
			return true
		}
	}
	return strings.Contains(text, "```")
}

func callbackAgentName(ctx agent.CallbackContext) string {
	if ctx == nil {
		return ""
	}
	return strings.TrimSpace(ctx.AgentName())
}

func estimateTextTokens(text string) int {
	return estimateTokens(text)
}

func redactContent(content *genai.Content) *genai.Content {
	if content == nil {
		return nil
	}
	parts := make([]*genai.Part, 0, len(content.Parts))
	changed := false
	for _, part := range content.Parts {
		if part == nil {
			continue
		}
		next := *part
		if next.Text != "" {
			masked := redactText(next.Text)
			if masked != next.Text {
				changed = true
				next.Text = masked
			}
		}
		parts = append(parts, &next)
	}
	if !changed {
		return content
	}
	return &genai.Content{Role: content.Role, Parts: parts}
}

var redactionPatterns = []struct {
	re          *regexp.Regexp
	replacement string
}{
	{regexp.MustCompile(`(?i)(sk-[a-z0-9][a-z0-9_\-]{12,})`), "[REDACTED_API_KEY]"},
	{regexp.MustCompile(`(?i)(api[_-]?key|access[_-]?token|refresh[_-]?token|token|secret|password)\s*[:=]\s*['"]?[^'"\s,;]+`), "$1=[REDACTED_SECRET]"},
	{regexp.MustCompile(`(?i)(postgres|mysql|mongodb|redis)://[^\s'"<>]+`), "[REDACTED_CONNECTION_STRING]"},
	{regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`), "[REDACTED_EMAIL]"},
	{regexp.MustCompile(`\b(?:\+?86[-\s]?)?1[3-9]\d{9}\b`), "[REDACTED_PHONE]"},
}

func redactText(text string) string {
	masked := text
	for _, pattern := range redactionPatterns {
		masked = pattern.re.ReplaceAllString(masked, pattern.replacement)
	}
	return masked
}

func llmRequestPreview(req *model.LLMRequest) string {
	if req == nil {
		return ""
	}
	parts := make([]string, 0, len(req.Contents))
	for _, content := range req.Contents {
		if text := contentText(content); text != "" {
			parts = append(parts, text)
		}
	}
	return redactText(strings.Join(parts, "\n"))
}

func previewForPlugin(text string, limit int) string {
	text = strings.TrimSpace(redactText(text))
	if limit <= 0 || len([]rune(text)) <= limit {
		return text
	}
	runes := []rune(text)
	return string(runes[:limit]) + "..."
}

func mustJSON(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%v", value)
	}
	return string(data)
}

func highRiskToolReason(toolName string, args map[string]any) string {
	name := strings.ToLower(strings.TrimSpace(toolName))
	argsText := strings.ToLower(mustJSON(args))
	riskyNames := []string{"delete", "remove", "unlink", "exec", "shell", "bash", "powershell", "sql_exec", "db_write", "drop", "truncate"}
	for _, item := range riskyNames {
		if strings.Contains(name, item) {
			return "tool name matches high-risk operation " + item
		}
	}
	riskyArgs := []string{"rm -rf", "del /", "remove-item", "drop table", "truncate table", "delete from", "chmod 777", "format "}
	for _, item := range riskyArgs {
		if strings.Contains(argsText, item) {
			return "tool arguments match high-risk pattern " + item
		}
	}
	return ""
}

func policyBlockedToolResult(message string, toolName string) map[string]any {
	return map[string]any{
		"status":  "blocked",
		"action":  "permission_denied",
		"tool":    toolName,
		"message": message,
	}
}

func outputPolicyViolation(text string, blockedPatterns []string) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return ""
	}
	if redactText(trimmed) != trimmed {
		return "sensitive data leak detected"
	}
	lower := strings.ToLower(trimmed)
	patterns := []string{"rm -rf", "drop table", "truncate table", "delete from", "format c:", "chmod 777", "curl ", " | sh", "powershell -enc"}
	patterns = append(patterns, blockedPatterns...)
	for _, pattern := range patterns {
		if strings.Contains(lower, strings.ToLower(pattern)) {
			return "dangerous command pattern " + pattern
		}
	}
	return ""
}

func isSkillTool(toolName string, args map[string]any) bool {
	name := strings.ToLower(strings.TrimSpace(toolName))
	if strings.Contains(name, "skill") {
		return true
	}
	for key, value := range args {
		key = strings.ToLower(key)
		if strings.Contains(key, "skill") {
			return true
		}
		if text, ok := value.(string); ok && strings.Contains(strings.ToLower(text), "skill") {
			return true
		}
	}
	return false
}

func toolInvocationKey(toolName string, args map[string]any) string {
	hash := sha256.Sum256([]byte(toolName + ":" + mustJSON(args)))
	return hex.EncodeToString(hash[:])
}

func envInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envBool(key string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return envBoolValue(value, fallback)
}

func envSet(key string) map[string]bool {
	result := map[string]bool{}
	for _, item := range strings.Split(os.Getenv(key), ",") {
		item = strings.ToLower(strings.TrimSpace(item))
		if item != "" {
			result[item] = true
		}
	}
	return result
}

func envMap(key string) map[string]string {
	result := map[string]string{}
	for _, item := range strings.Split(os.Getenv(key), ",") {
		parts := strings.SplitN(item, ":", 2)
		if len(parts) != 2 {
			continue
		}
		name := strings.TrimSpace(parts[0])
		modelName := strings.TrimSpace(parts[1])
		if name != "" && modelName != "" {
			result[name] = modelName
		}
	}
	return result
}

func configString(config map[string]any, field string, envKey string) string {
	if value, ok := config[field]; ok {
		if text, ok := value.(string); ok {
			return strings.TrimSpace(text)
		}
	}
	return strings.TrimSpace(os.Getenv(envKey))
}

func configInt(config map[string]any, field string, envKey string, fallback int) int {
	if value, ok := config[field]; ok {
		switch typed := value.(type) {
		case float64:
			return int(typed)
		case int:
			return typed
		case string:
			if parsed, err := strconv.Atoi(strings.TrimSpace(typed)); err == nil {
				return parsed
			}
		}
	}
	return envInt(envKey, fallback)
}

func configBool(config map[string]any, field string, envKey string, fallback bool) bool {
	if value, ok := config[field]; ok {
		switch typed := value.(type) {
		case bool:
			return typed
		case string:
			return envBoolValue(typed, fallback)
		}
	}
	return envBool(envKey, fallback)
}

func configSet(config map[string]any, field string, envKey string) map[string]bool {
	result := map[string]bool{}
	for _, item := range configStringSlice(config, field) {
		item = strings.ToLower(strings.TrimSpace(item))
		if item != "" {
			result[item] = true
		}
	}
	if len(result) > 0 {
		return result
	}
	return envSet(envKey)
}

func configStringSlice(config map[string]any, field string) []string {
	value, ok := config[field]
	if !ok {
		return nil
	}
	switch typed := value.(type) {
	case []any:
		items := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
				items = append(items, strings.TrimSpace(text))
			}
		}
		return items
	case []string:
		return typed
	case string:
		return splitCSV(typed)
	default:
		return nil
	}
}

func configMap(config map[string]any, field string, envKey string) map[string]string {
	if value, ok := config[field]; ok {
		result := map[string]string{}
		if raw, ok := value.(map[string]any); ok {
			for key, item := range raw {
				if text, ok := item.(string); ok && strings.TrimSpace(key) != "" && strings.TrimSpace(text) != "" {
					result[strings.TrimSpace(key)] = strings.TrimSpace(text)
				}
			}
		}
		if raw, ok := value.(map[string]string); ok {
			for key, text := range raw {
				if strings.TrimSpace(key) != "" && strings.TrimSpace(text) != "" {
					result[strings.TrimSpace(key)] = strings.TrimSpace(text)
				}
			}
		}
		if len(result) > 0 {
			return result
		}
	}
	return envMap(envKey)
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	items := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			items = append(items, part)
		}
	}
	return items
}

func envBoolValue(value string, fallback bool) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}
