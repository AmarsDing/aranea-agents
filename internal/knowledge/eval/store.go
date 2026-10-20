package eval

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

// LoadEvalSet reads a JSON EvalSet from path and validates it. Parsing is
// strict (unknown fields rejected) so a typo like "expected_chunk_id" fails
// loudly instead of silently dropping a case's expectations into vacuous
// success. LoadRunReport stays permissive for forward compatibility with
// snapshots written by newer binaries.
func LoadEvalSet(path string) (EvalSet, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return EvalSet{}, fmt.Errorf("eval: read set %s: %w", path, err)
	}
	var set EvalSet
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&set); err != nil {
		return EvalSet{}, fmt.Errorf("eval: parse set %s: %w", path, err)
	}
	if err := validateSet(set); err != nil {
		return EvalSet{}, fmt.Errorf("eval: validate set %s: %w", path, err)
	}
	return set, nil
}

// SaveEvalSet writes set as indented JSON to path (0o644), validating first.
func SaveEvalSet(path string, set EvalSet) error {
	if err := validateSet(set); err != nil {
		return fmt.Errorf("eval: validate set: %w", err)
	}
	raw, err := json.MarshalIndent(set, "", "  ")
	if err != nil {
		return fmt.Errorf("eval: encode set: %w", err)
	}
	if err := os.WriteFile(path, append(raw, '\n'), 0o644); err != nil {
		return fmt.Errorf("eval: write set %s: %w", path, err)
	}
	return nil
}

// LoadRunReport reads a JSON RunReport (e.g. the committed gold snapshot) from path.
func LoadRunReport(path string) (RunReport, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return RunReport{}, fmt.Errorf("eval: read report %s: %w", path, err)
	}
	var report RunReport
	if err := json.Unmarshal(raw, &report); err != nil {
		return RunReport{}, fmt.Errorf("eval: parse report %s: %w", path, err)
	}
	return report, nil
}

// SaveRunReport writes report as indented JSON to path (0o644), matching the
// SaveEvalSet on-disk convention (trailing newline for clean diffs).
func SaveRunReport(path string, report RunReport) error {
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("eval: encode report: %w", err)
	}
	if err := os.WriteFile(path, append(raw, '\n'), 0o644); err != nil {
		return fmt.Errorf("eval: write report %s: %w", path, err)
	}
	return nil
}

// validateSet requires a non-empty name, non-empty unique case ids, and a
// non-empty query per case.
func validateSet(set EvalSet) error {
	if strings.TrimSpace(set.Name) == "" {
		return errors.New("name is required")
	}
	seen := make(map[string]struct{}, len(set.Cases))
	for i, c := range set.Cases {
		if strings.TrimSpace(c.ID) == "" {
			return fmt.Errorf("case %d: id is required", i)
		}
		if _, ok := seen[c.ID]; ok {
			return fmt.Errorf("case %d: duplicate id %q", i, c.ID)
		}
		seen[c.ID] = struct{}{}
		if strings.TrimSpace(c.Query) == "" {
			return fmt.Errorf("case %q: query is required", c.ID)
		}
	}
	return nil
}
