package client

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	knowledgev1 "aranea-agents/api/kratos/knowledge/v1"
)

// ListCollections calls GET /v1/knowledge/collections.
func (c *Client) ListCollections(ctx context.Context, limit, offset int32) (*knowledgev1.ListCollectionsResponse, error) {
	params := url.Values{}
	if limit > 0 {
		params.Set("limit", fmt.Sprintf("%d", limit))
	}
	if offset > 0 {
		params.Set("offset", fmt.Sprintf("%d", offset))
	}
	path := "/v1/knowledge/collections"
	if len(params) > 0 {
		path += "?" + params.Encode()
	}
	resp := &knowledgev1.ListCollectionsResponse{}
	if err := c.Do(ctx, http.MethodGet, path, nil, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// GetCollection calls GET /v1/knowledge/collections/{id}.
func (c *Client) GetCollection(ctx context.Context, id string) (*knowledgev1.KnowledgeCollection, error) {
	resp := &knowledgev1.KnowledgeCollection{}
	if err := c.Do(ctx, http.MethodGet, "/v1/knowledge/collections/"+id, nil, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// CreateCollection calls POST /v1/knowledge/collections.
func (c *Client) CreateCollection(ctx context.Context, req *knowledgev1.CreateCollectionRequest) (*knowledgev1.KnowledgeCollection, error) {
	resp := &knowledgev1.KnowledgeCollection{}
	if err := c.Do(ctx, http.MethodPost, "/v1/knowledge/collections", req, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// DeleteCollection calls DELETE /v1/knowledge/collections/{id}.
func (c *Client) DeleteCollection(ctx context.Context, id string) error {
	return c.Do(ctx, http.MethodDelete, "/v1/knowledge/collections/"+id, nil, nil)
}

// ListDocuments calls GET /v1/knowledge/documents.
func (c *Client) ListDocuments(ctx context.Context, collectionID string, limit, offset int32) (*knowledgev1.ListDocumentsResponse, error) {
	params := url.Values{}
	if collectionID != "" {
		params.Set("collection_id", collectionID)
	}
	if limit > 0 {
		params.Set("limit", fmt.Sprintf("%d", limit))
	}
	if offset > 0 {
		params.Set("offset", fmt.Sprintf("%d", offset))
	}
	path := "/v1/knowledge/documents"
	if len(params) > 0 {
		path += "?" + params.Encode()
	}
	resp := &knowledgev1.ListDocumentsResponse{}
	if err := c.Do(ctx, http.MethodGet, path, nil, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// GetDocumentContent calls GET /v1/knowledge/documents/{id}/content.
func (c *Client) GetDocumentContent(ctx context.Context, id string) (*knowledgev1.DocumentContent, error) {
	resp := &knowledgev1.DocumentContent{}
	if err := c.Do(ctx, http.MethodGet, "/v1/knowledge/documents/"+id+"/content", nil, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// DeleteDocument calls DELETE /v1/knowledge/documents/{id}.
func (c *Client) DeleteDocument(ctx context.Context, id string) error {
	return c.Do(ctx, http.MethodDelete, "/v1/knowledge/documents/"+id, nil, nil)
}

// SearchKnowledge calls POST /v1/knowledge/search.
func (c *Client) SearchKnowledge(ctx context.Context, req *knowledgev1.SearchRequest) (*knowledgev1.SearchResponse, error) {
	resp := &knowledgev1.SearchResponse{}
	if err := c.Do(ctx, http.MethodPost, "/v1/knowledge/search", req, resp); err != nil {
		return nil, err
	}
	return resp, nil
}
