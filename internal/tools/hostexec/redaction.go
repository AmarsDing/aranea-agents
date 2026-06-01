package hostexec

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

const (
	redactedValue      = "[REDACTED]"
	redactedNameFormat = "[REDACTED:%s]"
	minSensitiveValueLength = 6
)

var (
	sensitiveEnvNamePattern = regexp.MustCompile(
		`(?i)\b[A-Z0-9_]*(TOKEN|SECRET|PASSWORD|PASSWD|API_KEY|ACCESS_KEY|PRIVATE_KEY)[A-Z0-9_]*\b`,
	)

	sensitiveAssignmentPattern = regexp.MustCompile(
		`^\s*((?:export|declare -x)\s+)?([A-Za-z_][A-Za-z0-9_]*)(\s*=\s*)(.*)$`,
	)

	sensitiveColonPattern = regexp.MustCompile(
		`^\s*([{"']?\s*)([A-Za-z_][A-Za-z0-9_]*)(["']?\s*:\s*)(.*)$`,
	)

	sensitiveInlineAssignPattern = regexp.MustCompile(
		`(?i)(?:^|[\s;|&])(?:export\s+)?([A-Za-z_][A-Za-z0-9_]*)=("[^"]*"|'[^']*'|[^\s;|&]+)`,
	)
)

type sensitiveValue struct {
	Name       string
	Value      string
	AllowShort bool
}

func RedactOutput(env map[string]string, output string) string {
	if strings.TrimSpace(output) == "" {
		return output
	}
	redacted := redactSensitiveKeyValueLines(output)
	return redactSensitiveValues(redacted, knownSensitiveValues(env))
}

func knownSensitiveValues(env map[string]string) []sensitiveValue {
	byName := make(map[string]sensitiveValue)
	addSensitiveEnvValues(byName, env)
	if len(byName) == 0 {
		return nil
	}

	out := make([]sensitiveValue, 0, len(byName))
	for _, item := range byName {
		if !item.AllowShort && len(item.Value) < minSensitiveValueLength {
			continue
		}
		out = append(out, item)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if len(out[i].Value) == len(out[j].Value) {
			return out[i].Name < out[j].Name
		}
		return len(out[i].Value) > len(out[j].Value)
	})
	return out
}

func addSensitiveEnvValues(out map[string]sensitiveValue, env map[string]string) {
	for name, value := range env {
		if !isSensitiveEnvName(name) {
			continue
		}
		if strings.TrimSpace(value) == "" {
			continue
		}
		out[strings.ToUpper(name)] = sensitiveValue{
			Name:       strings.ToUpper(name),
			Value:      value,
			AllowShort: true,
		}
	}
}

func redactSensitiveValues(output string, values []sensitiveValue) string {
	redacted := output
	for _, item := range values {
		if item.Value == "" {
			continue
		}
		redacted = strings.ReplaceAll(redacted, item.Value, redactedName(item.Name))
	}
	return redacted
}

func redactSensitiveKeyValueLines(output string) string {
	lines := strings.SplitAfter(output, "\n")
	for i, line := range lines {
		hasNewline := strings.HasSuffix(line, "\n")
		raw := strings.TrimSuffix(line, "\n")
		lines[i] = redactSensitiveKeyValueLine(raw)
		if hasNewline {
			lines[i] += "\n"
		}
	}
	return strings.Join(lines, "")
}

func redactSensitiveKeyValueLine(line string) string {
	if redacted, ok := redactAssignmentLine(line); ok {
		return redacted
	}
	if redacted, ok := redactColonLine(line); ok {
		return redacted
	}
	return line
}

func redactAssignmentLine(line string) (string, bool) {
	match := sensitiveAssignmentPattern.FindStringSubmatch(line)
	if len(match) != 5 {
		return "", false
	}
	if !isSensitiveEnvName(match[2]) {
		return "", false
	}
	return match[1] + match[2] + match[3] + redactedStructuredValue(match[4]), true
}

func redactColonLine(line string) (string, bool) {
	match := sensitiveColonPattern.FindStringSubmatch(line)
	if len(match) != 5 {
		return "", false
	}
	if !isSensitiveEnvName(match[2]) {
		return "", false
	}
	return match[1] + match[2] + match[3] + redactedStructuredValue(match[4]), true
}

func redactedStructuredValue(raw string) string {
	trimmedRight := strings.TrimRight(raw, " \t")
	suffix := raw[len(trimmedRight):]
	body := trimmedRight
	trailing := ""

	if strings.HasSuffix(body, ",") {
		body = strings.TrimSpace(strings.TrimSuffix(body, ","))
		trailing = ","
	}

	switch {
	case hasWrappedQuotes(body, '"'):
		return `"` + redactedValue + `"` + trailing + suffix
	case hasWrappedQuotes(body, '\''):
		return `'` + redactedValue + `'` + trailing + suffix
	default:
		return redactedValue + trailing + suffix
	}
}

func hasWrappedQuotes(value string, quote byte) bool {
	if len(value) < 2 {
		return false
	}
	return value[0] == quote && value[len(value)-1] == quote
}

func isSensitiveEnvName(name string) bool {
	return sensitiveEnvNamePattern.MatchString(name)
}

func redactedName(name string) string {
	return fmt.Sprintf(redactedNameFormat, strings.ToUpper(strings.TrimSpace(name)))
}
