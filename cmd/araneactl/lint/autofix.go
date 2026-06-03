package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// RunAutoFix runs all auto-fix tools on the repository.
func RunAutoFix(root string) error {
	abs, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("autofix: cannot resolve root: %w", err)
	}

	fmt.Println("araneactl lint --fix: running auto-fix tools...")

	// Go fix chain
	goFixes := []struct {
		name string
		cmd  string
		args []string
	}{
		{"gofmt", "gofmt", []string{"-w", "."}},
		{"goimports", "goimports", []string{"-w", "."}},
	}

	for _, fix := range goFixes {
		if _, err := exec.LookPath(fix.cmd); err != nil {
			fmt.Printf("  [SKIP] %s: not found in PATH\n", fix.name)
			continue
		}
		cmd := exec.Command(fix.cmd, fix.args...)
		cmd.Dir = abs
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		fmt.Printf("  [RUN] %s\n", fix.name)
		if err := cmd.Run(); err != nil {
			fmt.Printf("  [WARN] %s: %v\n", fix.name, err)
		} else {
			fmt.Printf("  [OK]  %s\n", fix.name)
		}
	}

	// golangci-lint --fix
	if _, err := exec.LookPath("golangci-lint"); err == nil {
		cmd := exec.Command("golangci-lint", "run", "--fix", "./...")
		cmd.Dir = abs
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		fmt.Println("  [RUN] golangci-lint --fix")
		cmd.Run() // golangci-lint may return non-zero for remaining issues
		fmt.Println("  [OK]  golangci-lint --fix")
	}

	// Frontend fix chain
	webDir := filepath.Join(abs, "web")
	if _, err := os.Stat(webDir); err == nil {
		pnpmCmd := "pnpm"
		if runtime.GOOS == "windows" {
			pnpmCmd = "pnpm.cmd"
		}

		// ESLint --fix
		cmd := exec.Command(pnpmCmd, "exec", "eslint", "--fix", "src/**/*.{ts,vue}")
		cmd.Dir = webDir
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		fmt.Println("  [RUN] eslint --fix (web)")
		cmd.Run()
		fmt.Println("  [OK]  eslint --fix (web)")

		// Stylelint --fix
		cmd = exec.Command(pnpmCmd, "exec", "stylelint", "--fix", "src/**/*.{vue,css}", "--config", ".stylelintrc.json")
		cmd.Dir = webDir
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		fmt.Println("  [RUN] stylelint --fix (web)")
		cmd.Run()
		fmt.Println("  [OK]  stylelint --fix (web)")
	}

	fmt.Println("araneactl lint --fix: done")
	return nil
}
