package agent

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"unicode"
)

// Shell confirmation classifier (E1). Catalog still marks shell_exec as
// RequiresConfirmation; this layer skips the first card for read-only /
// lint commands and keeps unknown + destructive commands on the confirm
// path. Grants still bypass danger (unlike computer-use danger words).

const (
	confirmReasonShellSafe   = "shell_safe"
	confirmReasonShellDanger = "shell_danger"
)

type shellConfirmClass int

const (
	shellClassUnknown shellConfirmClass = iota
	shellClassSafe
	shellClassDanger
)

var shellExecRuntimeNames = map[string]bool{
	"shell_exec":            true,
	"exec_command":          true,
	"hostexec_exec_command": true,
	"shell":                 true,
}

var shellDangerBins = map[string]bool{
	"rm": true, "rmdir": true, "rd": true, "del": true, "erase": true,
	"ri": true, "remove-item": true, "remove-itemproperty": true,
	"format": true, "mkfs": true, "dd": true,
	"sudo": true, "su": true, "chmod": true, "chown": true, "chgrp": true,
	"curl": true, "wget": true, "invoke-webrequest": true, "irm": true,
	"iex": true, "invoke-expression": true, "start-process": true,
	"shutdown": true, "reboot": true, "halt": true, "poweroff": true,
	"kill": true, "taskkill": true, "diskpart": true, "cipher": true,
	"msiexec": true, "reg": true, "sc": true,
}

var shellSafeBins = map[string]bool{
	"rg": true, "grep": true, "ls": true, "dir": true, "pwd": true,
	"cat": true, "head": true, "tail": true, "wc": true, "type": true,
	"whoami": true, "hostname": true, "uname": true,
}

var shellLintBins = map[string]bool{
	"golangci-lint": true, "gofmt": true, "goimports": true,
	"staticcheck": true, "eslint": true, "vet": true,
}

var gitSafeSubs = map[string]bool{
	"status": true, "diff": true, "log": true, "show": true,
	"rev-parse": true, "describe": true, "ls-files": true,
	"blame": true, "shortlog": true,
}

var gitDangerSubs = map[string]bool{
	"push": true, "reset": true, "clean": true, "checkout": true,
	"switch": true, "rebase": true, "commit": true, "merge": true,
	"tag": true, "stash": true, "config": true, "remote": true,
	"submodule": true, "filter-branch": true, "update-index": true,
	"revert": true, "cherry-pick": true, "branch": true,
}

var goSafeSubs = map[string]bool{
	"test": true, "vet": true, "fmt": true, "list": true,
	"env": true, "version": true, "help": true, "doc": true,
}

func isShellExecRuntimeName(toolName string) bool {
	n := strings.TrimSpace(toolName)
	if n == "" {
		return false
	}
	if shellExecRuntimeNames[n] {
		return true
	}
	return strings.HasSuffix(n, "_exec_command") || strings.HasSuffix(n, "_shell_exec")
}

func extractShellCommand(args []byte) string {
	if len(args) == 0 {
		return ""
	}
	var m map[string]any
	if json.Unmarshal(args, &m) != nil {
		return ""
	}
	for _, key := range []string{"command", "cmd"} {
		switch v := m[key].(type) {
		case string:
			if s := strings.TrimSpace(v); s != "" {
				return s
			}
		case []any:
			parts := make([]string, 0, len(v))
			for _, item := range v {
				s, ok := item.(string)
				if !ok || strings.TrimSpace(s) == "" {
					return ""
				}
				parts = append(parts, strings.TrimSpace(s))
			}
			if len(parts) > 0 {
				return strings.Join(parts, " ")
			}
		}
	}
	return ""
}

func classifyShellForConfirm(toolName string, args []byte) (shellConfirmClass, bool) {
	if !isShellExecRuntimeName(toolName) {
		return shellClassUnknown, false
	}
	return classifyShellCommand(extractShellCommand(args)), true
}

func classifyShellCommand(cmd string) shellConfirmClass {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return shellClassUnknown
	}
	if strings.ContainsAny(cmd, "\n\r") {
		return shellClassUnknown
	}
	lower := strings.ToLower(cmd)
	if strings.ContainsAny(cmd, "|;`") || strings.Contains(cmd, "&&") ||
		strings.Contains(cmd, "$(") || strings.Contains(cmd, "${") {
		if shellHasDangerToken(lower) {
			return shellClassDanger
		}
		return shellClassUnknown
	}
	if strings.Contains(cmd, "&") {
		return shellClassUnknown
	}
	tokens := splitShellWords(cmd)
	if len(tokens) == 0 {
		return shellClassUnknown
	}
	bin := normalizeShellBin(tokens[0])
	if shellDangerBins[bin] {
		return shellClassDanger
	}
	switch bin {
	case "git":
		return classifyGit(tokens[1:])
	case "go":
		return classifyGo(tokens[1:])
	case "prettier":
		for _, a := range tokens[1:] {
			if a == "--write" || a == "-w" {
				return shellClassUnknown
			}
		}
		return shellClassSafe
	}
	if shellSafeBins[bin] || shellLintBins[bin] {
		return shellClassSafe
	}
	return shellClassUnknown
}

func classifyGit(args []string) shellConfirmClass {
	i := 0
	for i < len(args) {
		a := args[i]
		switch a {
		case "-C", "-c", "--git-dir", "--work-tree":
			i += 2
			continue
		}
		if strings.HasPrefix(a, "-") {
			i++
			continue
		}
		break
	}
	if i >= len(args) {
		return shellClassUnknown
	}
	sub := strings.ToLower(args[i])
	if gitDangerSubs[sub] {
		return shellClassDanger
	}
	if gitSafeSubs[sub] {
		return shellClassSafe
	}
	return shellClassUnknown
}

func classifyGo(args []string) shellConfirmClass {
	if len(args) == 0 {
		return shellClassUnknown
	}
	sub := strings.ToLower(args[0])
	if !goSafeSubs[sub] {
		return shellClassUnknown
	}
	for _, a := range args[1:] {
		al := strings.ToLower(a)
		if al == "-exec" || strings.HasPrefix(al, "-exec=") ||
			al == "-toolexec" || strings.HasPrefix(al, "-toolexec=") {
			return shellClassDanger
		}
	}
	return shellClassSafe
}

func normalizeShellBin(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	s = strings.Trim(s, `"'`)
	s = filepath.Base(s)
	s = strings.TrimSuffix(s, ".exe")
	s = strings.TrimSuffix(s, ".cmd")
	s = strings.TrimSuffix(s, ".bat")
	return s
}

func shellHasDangerToken(lower string) bool {
	for _, tok := range []string{
		" rm ", "rm ", " rmdir ", " del ", " sudo ", " curl ", " wget ",
		" git push", " git reset", " git clean", " git checkout",
		"invoke-webrequest", "invoke-expression",
	} {
		if strings.Contains(lower, tok) {
			return true
		}
	}
	return strings.HasPrefix(lower, "rm ") || lower == "rm"
}

func splitShellWords(cmd string) []string {
	var out []string
	var b strings.Builder
	inDouble := false
	for _, r := range cmd {
		switch {
		case r == '"':
			inDouble = !inDouble
		case r == '\'':
			// Unparsed single quotes: fail-safe (caller treats empty as unknown).
			if !inDouble {
				return nil
			}
			b.WriteRune(r)
		case unicode.IsSpace(r) && !inDouble:
			if b.Len() > 0 {
				out = append(out, b.String())
				b.Reset()
			}
		default:
			b.WriteRune(r)
		}
	}
	if inDouble {
		return nil
	}
	if b.Len() > 0 {
		out = append(out, b.String())
	}
	return out
}
