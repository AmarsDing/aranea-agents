package orgimport

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// LoadOptions controls how a spec file is loaded.
type LoadOptions struct {
	// ExtractViaAPI uses POST /v1/ai/refine (scope=spec_extract) to convert
	// markdown prose into a structured YAML spec. Set when the input file ends
	// in .md or when ForceExtract is true.
	ExtractViaAPI bool
	// APIURL is the base URL of the Aranea backend (e.g. http://localhost:8000).
	APIURL string
	// APIToken is the optional Bearer token for authenticated requests.
	APIToken string
	// Timeout for HTTP calls.
	Timeout time.Duration
}

// LoadSpec reads a YAML or Markdown spec file and returns a Spec.
// For markdown files, it calls the AI extraction endpoint to produce YAML first.
func LoadSpec(path string, opts LoadOptions) (*Spec, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("orgimport: read %s: %w", path, err)
	}
	content := string(raw)

	if opts.ExtractViaAPI || isMDFile(path) {
		content, err = extractSpecViaAPI(content, opts)
		if err != nil {
			return nil, fmt.Errorf("orgimport: LLM extract: %w", err)
		}
	}

	var spec Spec
	if err := yaml.Unmarshal([]byte(content), &spec); err != nil {
		return nil, fmt.Errorf("orgimport: yaml unmarshal: %w", err)
	}
	if spec.Version == 0 {
		spec.Version = 1
	}
	return &spec, nil
}

func isMDFile(path string) bool {
	return strings.HasSuffix(strings.ToLower(path), ".md") ||
		strings.HasSuffix(strings.ToLower(path), ".markdown")
}

// extractSpecViaAPI calls POST /v1/ai/refine with scope=spec_extract to convert
// markdown prose to a YAML org spec. The server-side refiner uses a special
// system prompt that produces structured YAML. PGO-4-IMP-03.
func extractSpecViaAPI(mdContent string, opts LoadOptions) (string, error) {
	if opts.APIURL == "" {
		return "", fmt.Errorf("APIURL is required for markdown extraction")
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	client := &http.Client{Timeout: timeout}

	reqBody := map[string]string{
		"scope":         "spec_extract",
		"original_text": mdContent,
		"target_mode":   "complete",
	}
	bodyBytes, _ := json.Marshal(reqBody)
	req, err := http.NewRequest("POST", opts.APIURL+"/v1/ai/refine", bytes.NewReader(bodyBytes))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if opts.APIToken != "" {
		req.Header.Set("Authorization", "Bearer "+opts.APIToken)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("refine API call failed: %w", err)
	}
	defer resp.Body.Close()

	bodyRaw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("refine API returned %d: %s", resp.StatusCode, string(bodyRaw))
	}

	var result struct {
		Refined string `json:"refined"`
	}
	if err := json.Unmarshal(bodyRaw, &result); err != nil {
		return "", fmt.Errorf("decode refine response: %w", err)
	}
	// The LLM should output raw YAML; strip any markdown code fences.
	return stripCodeFence(result.Refined), nil
}

func stripCodeFence(s string) string {
	s = strings.TrimSpace(s)
	// Remove ```yaml ... ``` or ``` ... ```
	if strings.HasPrefix(s, "```") {
		lines := strings.Split(s, "\n")
		if len(lines) >= 2 {
			lines = lines[1:] // remove first ```... line
		}
		if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "```" {
			lines = lines[:len(lines)-1]
		}
		s = strings.Join(lines, "\n")
	}
	return strings.TrimSpace(s)
}
