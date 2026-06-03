package client

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"

	packv1 "aranea-agents/api/kratos/pack/v1"
)

// PackExportResult holds the export response data.
type PackExportResult struct {
	Data []byte
	Name string
	Kind string
}

// PackExport calls POST /v1/pack/export.
func (c *Client) PackExport(ctx context.Context, kind, ref string) (*PackExportResult, error) {
	reqBody := fmt.Sprintf(`{"kind":"%s","ref":"%s"}`, escapeJSON(kind), escapeJSON(ref))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Base+"/v1/pack/export", bytes.NewReader([]byte(reqBody)))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.UA)
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}

	resp, err := c.Doer.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	var pb packv1.ExportPackResponse
	if err := unmarshalOpts.Unmarshal(body, &pb); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}

	return &PackExportResult{
		Data: pb.Data,
		Name: pb.Name,
		Kind: pb.Kind,
	}, nil
}

// PackImportResult holds the import response.
type PackImportResult struct {
	TaxonomyNodes    int
	AgentsCreated    int
	AgentsUpdated    int
	AgentsSkipped    int
	GraphsCreated    int
	TeamsCreated     int
	TeamsUpdated     int
	TeamsSkipped     int
	ConflictStrategy string
	Failures         []PackImportFailure
}

// PackImportFailure records a single import failure.
type PackImportFailure struct {
	EntityType string
	Key        string
	Reason     string
}

// PackImport calls POST /v1/pack/import.
func (c *Client) PackImport(ctx context.Context, data []byte, strategy string) (*PackImportResult, error) {
	encoded := base64.StdEncoding.EncodeToString(data)
	reqBody := fmt.Sprintf(`{"data":"%s","conflict_strategy":"%s"}`, encoded, escapeJSON(strategy))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Base+"/v1/pack/import", bytes.NewReader([]byte(reqBody)))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.UA)
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}

	resp, err := c.Doer.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	var pb packv1.ImportPackResponse
	if err := unmarshalOpts.Unmarshal(body, &pb); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}

	result := &PackImportResult{
		TaxonomyNodes:    int(pb.TaxonomyNodes),
		AgentsCreated:    int(pb.AgentsCreated),
		AgentsUpdated:    int(pb.AgentsUpdated),
		AgentsSkipped:    int(pb.AgentsSkipped),
		GraphsCreated:    int(pb.GraphsCreated),
		TeamsCreated:     int(pb.TeamsCreated),
		TeamsUpdated:     int(pb.TeamsUpdated),
		TeamsSkipped:     int(pb.TeamsSkipped),
		ConflictStrategy: pb.ConflictStrategy,
	}
	for _, f := range pb.Failures {
		result.Failures = append(result.Failures, PackImportFailure{
			EntityType: f.EntityType,
			Key:        f.Key,
			Reason:     f.Reason,
		})
	}
	return result, nil
}

// PackValidateResult holds the validation response.
type PackValidateResult struct {
	Valid           bool
	Errors          []string
	MissingSkills   []string
	MissingFuncRefs []string
	Conflicts       []PackConflictItem
}

// PackConflictItem describes a conflict.
type PackConflictItem struct {
	EntityType string
	Key        string
}

// PackValidate calls POST /v1/pack/validate.
func (c *Client) PackValidate(ctx context.Context, data []byte) (*PackValidateResult, error) {
	encoded := base64.StdEncoding.EncodeToString(data)
	reqBody := fmt.Sprintf(`{"data":"%s"}`, encoded)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Base+"/v1/pack/validate", bytes.NewReader([]byte(reqBody)))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.UA)
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}

	resp, err := c.Doer.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	var pb packv1.ValidatePackResponse
	if err := unmarshalOpts.Unmarshal(body, &pb); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}

	result := &PackValidateResult{
		Valid:           pb.Valid,
		Errors:          pb.Errors,
		MissingSkills:   pb.MissingSkills,
		MissingFuncRefs: pb.MissingFuncRefs,
	}
	for _, ci := range pb.Conflicts {
		result.Conflicts = append(result.Conflicts, PackConflictItem{
			EntityType: ci.EntityType,
			Key:        ci.Key,
		})
	}
	return result, nil
}
