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
		redacted = replaceValueWithBoundary(redacted, item.Value, redactedName(item.Name))
	}
	return redacted
}

// replaceValueWithBoundary replaces occurrences of value that appear as whole words
// or within structured contexts (quotes, assignments), avoiding partial matches
// in unrelated text.
func replaceValueWithBoundary(output, value, replacement string) string {
	if len(value) >= minSensitiveValueLength {
		return replaceWithWordBoundary(output, value, replacement)
	}
	return strings.ReplaceAll(output, value, replacement)
}

// replaceWithWordBoundary replaces value occurrences that appear as whole words
// or within quoted contexts, without using regexp to avoid runtime compilation cost.
func replaceWithWordBoundary(output, value, replacement string) string {
	var b strings.Builder
	b.Grow(len(output))
	i := 0
	for {
		idx := strings.Index(output[i:], value)
		if idx < 0 {
			b.WriteString(output[i:])
			break
		}
		pos := i + idx
		beforeOK := pos == 0 || !isWordChar(output[pos-1])
		afterPos := pos + len(value)
		afterOK := afterPos >= len(output) || !isWordChar(output[afterPos])

		if beforeOK && afterOK {
			b.WriteString(output[i:pos])
			b.WriteString(replacement)
		} else {
			// Check for quoted context: "value" or 'value' or `value`
			inQuotes := false
			if pos > 0 && afterPos < len(output) {
				q := output[pos-1]
				if (q == '"' || q == '\'' || q == '`') && output[afterPos] == q {
					inQuotes = true
				}
			}
			if inQuotes {
				b.WriteString(output[i : pos-1])
				b.WriteString(replacement)
				i = afterPos + 1
				continue
			}
			b.WriteString(output[i : pos+len(value)])
		}
		i = pos + len(value)
	}
	return b.String()
}

func isWordChar(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '_'
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
