package monitor

import (
	"regexp"
	"strconv"
	"strings"
)

// CI log patterns — compiled once at package init.
var (
	// Go build errors: "file.go:line:col: message"
	reGoBuildError = regexp.MustCompile(`^([\w./\-]+\.go):(\d+):\d+:\s*(.+)$`)

	// Go test failures: "file.go:line: message" (indented under --- FAIL)
	reGoTestFailure = regexp.MustCompile(`^\s+([\w./\-]+\.go):(\d+):\s*(.+)$`)

	// Go lint errors: "file.go:line:col: message (linter)"
	reGoLintError = regexp.MustCompile(`^([\w./\-]+\.go):(\d+):\d+:\s*(.+)\s+\((\w+)\)$`)

	// Proto sync: "Proto generated files are out of date"
	reProtoSync = regexp.MustCompile(`(?i)proto generated files are out of date|generated files.*out of date|wire_gen\.go.*out of date`)

	// Runtime panic: "panic: message"
	reRuntimePanic = regexp.MustCompile(`^panic:\s*(.+)$`)

	// Runtime nil pointer: "runtime error: invalid memory address or nil pointer dereference"
	reRuntimeNilPointer = regexp.MustCompile(`(?i)nil pointer dereference|invalid memory address`)

	// Stack trace file reference: "	/path/file.go:line +0xoffset"
	reStackFile = regexp.MustCompile(`^\s+([\w./\-]+\.go):(\d+)`)

	// Generic connection refused
	reConnectionRefused = regexp.MustCompile(`(?i)connection refused|dial tcp.*connection refused`)
)

// ParseCILogs parses CI pipeline logs into FailureReport slices. The job
// parameter identifies the CI job name (e.g. "build", "test", "lint").
// It recognizes Go build errors, test failures, lint errors, and proto sync issues.
func ParseCILogs(logs string, job string) []*FailureReport {
	if strings.TrimSpace(logs) == "" {
		return nil
	}

	var reports []*FailureReport
	lines := strings.Split(logs, "\n")

	for _, line := range lines {
		// Try proto sync first (full-line match)
		if reProtoSync.MatchString(line) {
			fr := NewFailureReport()
			fr.Type = FailureTypeProtoSync
			fr.Source = "ci"
			fr.Job = job
			fr.Message = strings.TrimSpace(line)
			fr.Metadata["raw_line"] = line
			reports = append(reports, fr)
			continue
		}

		// Try lint error (has linter name in parentheses)
		if matches := reGoLintError.FindStringSubmatch(line); len(matches) >= 5 {
			fr := NewFailureReport()
			fr.Type = FailureTypeLint
			fr.Source = "ci"
			fr.Job = job
			fr.File = matches[1]
			fr.Line = mustAtoi(matches[2])
			fr.Message = matches[3]
			fr.ErrorCode = matches[4] // linter name
			fr.Metadata["raw_line"] = line
			reports = append(reports, fr)
			continue
		}

		// Try build error
		if matches := reGoBuildError.FindStringSubmatch(line); len(matches) >= 4 {
			// Skip if it's actually a test failure line (indented)
			if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
				continue
			}
			fr := NewFailureReport()
			fr.Type = FailureTypeBuild
			fr.Source = "ci"
			fr.Job = job
			fr.File = matches[1]
			fr.Line = mustAtoi(matches[2])
			fr.Message = matches[3]
			fr.Metadata["raw_line"] = line
			reports = append(reports, fr)
			continue
		}

		// Try test failure (indented lines under --- FAIL)
		if matches := reGoTestFailure.FindStringSubmatch(line); len(matches) >= 4 {
			fr := NewFailureReport()
			fr.Type = FailureTypeTest
			fr.Source = "ci"
			fr.Job = job
			fr.File = matches[1]
			fr.Line = mustAtoi(matches[2])
			fr.Message = matches[3]
			fr.Metadata["raw_line"] = line
			reports = append(reports, fr)
			continue
		}
	}

	return reports
}

// ParseRuntimeError parses a runtime error message (typically a panic or
// unhandled error) into a single FailureReport. The component parameter
// identifies the runtime component that produced the error.
func ParseRuntimeError(errMsg string, component string) *FailureReport {
	if strings.TrimSpace(errMsg) == "" {
		return nil
	}

	fr := NewFailureReport()
	fr.Type = FailureTypeRuntime
	fr.Source = "runtime"
	fr.Job = component

	lines := strings.Split(errMsg, "\n")

	// Extract stack trace and file info
	var stackLines []string
	var firstFile string
	var firstLine int

	for _, line := range lines {
		// Check for panic message
		if matches := reRuntimePanic.FindStringSubmatch(line); len(matches) >= 2 {
			fr.Message = matches[1]
			if reRuntimeNilPointer.MatchString(line) {
				fr.ErrorCode = "nil_pointer_dereference"
			}
			continue
		}

		// Check for nil pointer in non-panic lines
		if reRuntimeNilPointer.MatchString(line) && fr.ErrorCode == "" {
			fr.ErrorCode = "nil_pointer_dereference"
			if fr.Message == "" {
				fr.Message = strings.TrimSpace(line)
			}
		}

		// Check for connection refused
		if reConnectionRefused.MatchString(line) && fr.ErrorCode == "" {
			fr.ErrorCode = "connection_refused"
			if fr.Message == "" {
				fr.Message = strings.TrimSpace(line)
			}
		}

		// Extract file references from stack trace
		if matches := reStackFile.FindStringSubmatch(line); len(matches) >= 3 {
			stackLines = append(stackLines, line)
			if firstFile == "" {
				firstFile = matches[1]
				firstLine = mustAtoi(matches[2])
			}
		}
	}

	// Set file info from first stack frame
	if firstFile != "" {
		fr.File = firstFile
		fr.Line = firstLine
	}

	// Set stack trace
	if len(stackLines) > 0 {
		fr.StackTrace = strings.Join(stackLines, "\n")
	}

	// Fallback: if no specific message was extracted, use the first line
	if fr.Message == "" && len(lines) > 0 {
		fr.Message = strings.TrimSpace(lines[0])
	}

	return fr
}

// mustAtoi converts s to int, returning 0 on failure.
func mustAtoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}
