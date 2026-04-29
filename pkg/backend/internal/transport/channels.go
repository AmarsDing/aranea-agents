package transport

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"arenea/backend/internal/domain"
)

type channelResourceRequest struct {
	Key          string                          `json:"key"`
	Name         string                          `json:"name"`
	Description  string                          `json:"description"`
	Status       string                          `json:"status"`
	Enabled      *bool                           `json:"enabled"`
	SortOrder    int                             `json:"sort_order"`
	ConfigJSON   string                          `json:"config_json"`
	MetadataJSON string                          `json:"metadata_json"`
	Credentials  []domain.ChannelCredentialInput `json:"credentials"`
}

func (h *HTTPHandler) handleChannelCatalog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	writeJSON(w, http.StatusOK, listResponse[domain.ChannelCatalogItem]{Items: h.channelSvc.Catalog()})
}

func (h *HTTPHandler) handleChannels(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items, err := h.channelSvc.List()
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, listResponse[domain.PlatformResource]{Items: items})
	case http.MethodPost:
		in, credentials, err := decodeChannelResourceRequest(r, nil)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		created, err := h.channelSvc.Create(in, credentials)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		_ = h.auditSvc.Log("create", "channels", created.ID, r.Header.Get("X-Request-Id"), created.Key)
		writeJSON(w, http.StatusCreated, created)
	default:
		methodNotAllowed(w)
	}
}

func (h *HTTPHandler) handleChannelByID(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/channels/"), "/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 || parts[0] == "" {
		writeErr(w, http.StatusBadRequest, errors.New("channel id is required"))
		return
	}
	id := parts[0]
	if len(parts) == 1 {
		h.handleChannelItem(w, r, id)
		return
	}
	switch parts[1] {
	case "toggle":
		h.handleChannelToggle(w, r, id)
	case "test":
		h.handleChannelTest(w, r, id)
	case "credentials":
		if len(parts) == 3 {
			h.handleChannelCredentialByKey(w, r, id, parts[2])
			return
		}
		h.handleChannelCredentials(w, r, id)
	case "deliveries":
		h.handleChannelDeliveries(w, r, id)
	default:
		methodNotAllowed(w)
	}
}

func (h *HTTPHandler) handleChannelItem(w http.ResponseWriter, r *http.Request, id string) {
	switch r.Method {
	case http.MethodGet:
		row, err := h.channelSvc.Get(id)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, row)
	case http.MethodPatch:
		current, err := h.channelSvc.Get(id)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		in, credentials, err := decodeChannelResourceRequest(r, &current)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		updated, err := h.channelSvc.Update(id, in, credentials)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		_ = h.auditSvc.Log("update", "channels", updated.ID, r.Header.Get("X-Request-Id"), updated.Key)
		writeJSON(w, http.StatusOK, updated)
	case http.MethodDelete:
		if err := h.channelSvc.Delete(id); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		_ = h.auditSvc.Log("delete", "channels", id, r.Header.Get("X-Request-Id"), "")
		w.WriteHeader(http.StatusNoContent)
	default:
		methodNotAllowed(w)
	}
}

func (h *HTTPHandler) handleChannelToggle(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	updated, err := h.channelSvc.Toggle(id, body.Enabled)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	_ = h.auditSvc.Log("toggle", "channels", updated.ID, r.Header.Get("X-Request-Id"), updated.Key)
	writeJSON(w, http.StatusOK, updated)
}

func (h *HTTPHandler) handleChannelTest(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	result, err := h.channelSvc.Test(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *HTTPHandler) handleChannelCredentials(w http.ResponseWriter, r *http.Request, id string) {
	switch r.Method {
	case http.MethodGet:
		items, err := h.channelSvc.ListCredentials(id)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, listResponse[domain.ChannelCredential]{Items: items})
	case http.MethodPut:
		var body struct {
			Credentials []domain.ChannelCredentialInput `json:"credentials"`
		}
		if !decodeBody(w, r, &body) {
			return
		}
		items, err := h.channelSvc.UpsertCredentials(id, body.Credentials)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		_ = h.auditSvc.Log("credentials.update", "channels", id, r.Header.Get("X-Request-Id"), "")
		writeJSON(w, http.StatusOK, listResponse[domain.ChannelCredential]{Items: items})
	default:
		methodNotAllowed(w)
	}
}

func (h *HTTPHandler) handleChannelCredentialByKey(w http.ResponseWriter, r *http.Request, id string, key string) {
	if r.Method != http.MethodDelete {
		methodNotAllowed(w)
		return
	}
	if err := h.channelSvc.DeleteCredential(id, key); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	_ = h.auditSvc.Log("credentials.delete", "channels", id, r.Header.Get("X-Request-Id"), key)
	w.WriteHeader(http.StatusNoContent)
}

func (h *HTTPHandler) handleChannelDeliveries(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	items, err := h.channelSvc.ListDeliveries(id, intQueryParam(r, "limit", 50))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, listResponse[domain.ChannelDelivery]{Items: items})
}

func decodeChannelResourceRequest(r *http.Request, current *domain.PlatformResource) (domain.PlatformResource, []domain.ChannelCredentialInput, error) {
	var body channelResourceRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return domain.PlatformResource{}, nil, err
	}
	row := domain.PlatformResource{
		Resource:     "channels",
		Key:          body.Key,
		Name:         body.Name,
		Description:  body.Description,
		Status:       body.Status,
		SortOrder:    body.SortOrder,
		ConfigJSON:   body.ConfigJSON,
		MetadataJSON: body.MetadataJSON,
	}
	if current != nil {
		row = *current
		if body.Key != "" {
			row.Key = body.Key
		}
		if body.Name != "" {
			row.Name = body.Name
		}
		row.Description = body.Description
		if body.Status != "" {
			row.Status = body.Status
		}
		row.SortOrder = body.SortOrder
		if body.ConfigJSON != "" {
			row.ConfigJSON = body.ConfigJSON
		}
		if body.MetadataJSON != "" {
			row.MetadataJSON = body.MetadataJSON
		}
	}
	if body.Enabled != nil {
		row.Enabled = *body.Enabled
	} else if current == nil {
		row.Enabled = true
	}
	return row, body.Credentials, nil
}
