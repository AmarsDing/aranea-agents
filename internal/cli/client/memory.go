package client

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	memoryv1 "aranea-agents/api/kratos/memory/v1"
)

// ListMemoryFacts calls GET /v1/memory/l3/facts.
func (c *Client) ListMemoryFacts(ctx context.Context, scopeType, scopeID, kind, status, keyword string, limit, offset int32) (*memoryv1.ListMemoryFactsResponse, error) {
	params := url.Values{}
	if scopeType != "" {
		params.Set("scope_type", scopeType)
	}
	if scopeID != "" {
		params.Set("scope_id", scopeID)
	}
	if kind != "" {
		params.Set("kind", kind)
	}
	if status != "" {
		params.Set("status", status)
	}
	if keyword != "" {
		params.Set("keyword", keyword)
	}
	if limit > 0 {
		params.Set("limit", fmt.Sprintf("%d", limit))
	}
	if offset > 0 {
		params.Set("offset", fmt.Sprintf("%d", offset))
	}
	path := "/v1/memory/l3/facts"
	if len(params) > 0 {
		path += "?" + params.Encode()
	}
	resp := &memoryv1.ListMemoryFactsResponse{}
	if err := c.Do(ctx, http.MethodGet, path, nil, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// ListCascadeProposals calls GET /v1/memory/cascade/proposals.
func (c *Client) ListCascadeProposals(ctx context.Context, agentID, status string, limit int32) (*memoryv1.ListCascadeProposalsResponse, error) {
	params := url.Values{}
	params.Set("agent_id", agentID)
	if status != "" {
		params.Set("status", status)
	}
	if limit > 0 {
		params.Set("limit", fmt.Sprintf("%d", limit))
	}
	resp := &memoryv1.ListCascadeProposalsResponse{}
	if err := c.Do(ctx, http.MethodGet, "/v1/memory/cascade/proposals?"+params.Encode(), nil, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// ApproveCascadeProposal calls POST /v1/memory/cascade/proposals/{id}/approve.
func (c *Client) ApproveCascadeProposal(ctx context.Context, id, reviewer string) (*memoryv1.CascadeProposal, error) {
	req := &memoryv1.ApproveCascadeProposalRequest{Id: id, Reviewer: reviewer}
	resp := &memoryv1.ApproveCascadeProposalResponse{}
	if err := c.Do(ctx, http.MethodPost, "/v1/memory/cascade/proposals/"+id+"/approve", req, resp); err != nil {
		return nil, err
	}
	return resp.Proposal, nil
}

// RejectCascadeProposal calls POST /v1/memory/cascade/proposals/{id}/reject.
func (c *Client) RejectCascadeProposal(ctx context.Context, id, reviewer, reason string) (*memoryv1.CascadeProposal, error) {
	req := &memoryv1.RejectCascadeProposalRequest{Id: id, Reviewer: reviewer, Reason: reason}
	resp := &memoryv1.RejectCascadeProposalResponse{}
	if err := c.Do(ctx, http.MethodPost, "/v1/memory/cascade/proposals/"+id+"/reject", req, resp); err != nil {
		return nil, err
	}
	return resp.Proposal, nil
}

// CompositeSearchMemories calls POST /v1/memory/search/composite.
func (c *Client) CompositeSearchMemories(ctx context.Context, req *memoryv1.CompositeSearchMemoriesRequest) (*memoryv1.CompositeSearchMemoriesResponse, error) {
	resp := &memoryv1.CompositeSearchMemoriesResponse{}
	if err := c.Do(ctx, http.MethodPost, "/v1/memory/search/composite", req, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// DebugMemoryRecall calls POST /v1/memory/recall/debug.
func (c *Client) DebugMemoryRecall(ctx context.Context, req *memoryv1.DebugMemoryRecallRequest) (*memoryv1.DebugMemoryRecallResponse, error) {
	resp := &memoryv1.DebugMemoryRecallResponse{}
	if err := c.Do(ctx, http.MethodPost, "/v1/memory/recall/debug", req, resp); err != nil {
		return nil, err
	}
	return resp, nil
}
