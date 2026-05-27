package ui

import (
	"bufio"
	"fmt"
	"strings"
)

// ConfirmYesNo shows a yes/no prompt and returns the user's choice.
// If the UI is not a TTY, it returns an error immediately.
func (u UI) ConfirmYesNo(prompt string, defaultYes bool) (bool, error) {
	if !u.IsTTY {
		return false, fmt.Errorf("non-interactive terminal; use --yes to skip confirmation")
	}
	suffix := " [y/N]: "
	if defaultYes {
		suffix = " [Y/n]: "
	}
	fmt.Fprintf(u.Out, "%s%s", prompt, suffix)

	scanner := bufio.NewScanner(u.In)
	if !scanner.Scan() {
		return false, scanner.Err()
	}
	answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
	switch answer {
	case "y", "yes":
		return true, nil
	case "n", "no":
		return false, nil
	case "":
		return defaultYes, nil
	default:
		return false, nil
	}
}

// Select shows a numbered list and returns the chosen index.
func (u UI) Select(prompt string, choices []string) (int, error) {
	if !u.IsTTY {
		return -1, fmt.Errorf("non-interactive terminal; use --decision flag instead")
	}
	fmt.Fprintln(u.Out, prompt)
	for i, c := range choices {
		fmt.Fprintf(u.Out, "  [%d] %s\n", i+1, c)
	}
	fmt.Fprint(u.Out, "Enter choice: ")

	scanner := bufio.NewScanner(u.In)
	if !scanner.Scan() {
		return -1, scanner.Err()
	}
	text := strings.TrimSpace(scanner.Text())
	var idx int
	if _, err := fmt.Sscanf(text, "%d", &idx); err != nil || idx < 1 || idx > len(choices) {
		return -1, fmt.Errorf("invalid choice %q", text)
	}
	return idx - 1, nil
}
