package transport

import (
	"errors"
	"net/http"
	"strings"

	"arenea/backend/internal/domain"
)

func (h *HTTPHandler) handlePlatformCollection(resource string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			items, err := h.platformSvc.List(resource)
			if err != nil {
				writeErr(w, http.StatusBadRequest, err)
				return
			}
			writeJSON(w, http.StatusOK, listResponse[domain.PlatformResource]{Items: items})
		case http.MethodPost:
			var in domain.PlatformResource
			if !decodeBody(w, r, &in) {
				return
			}
			created, err := h.platformSvc.Create(resource, in)
			if err != nil {
				writeErr(w, http.StatusBadRequest, err)
				return
			}
			_ = h.auditSvc.Log("create", resource, created.ID, r.Header.Get("X-Request-Id"), created.Key)
			writeJSON(w, http.StatusCreated, created)
		default:
			methodNotAllowed(w)
		}
	}
}

func (h *HTTPHandler) handlePlatformTree(resource string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		items, err := h.platformSvc.Tree(resource)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, listResponse[domain.PlatformResourceTreeNode]{Items: items})
	}
}

func (h *HTTPHandler) handlePlatformItem(resource string, prefix string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := idFromPath(r.URL.Path, prefix)
		if id == "" {
			writeErr(w, http.StatusBadRequest, errors.New("resource id is required"))
			return
		}
		if resource == "mcp-servers" && strings.HasSuffix(id, "/test") {
			if r.Method != http.MethodPost {
				methodNotAllowed(w)
				return
			}
			result, err := h.platformSvc.TestMCPServer(strings.TrimSuffix(id, "/test"))
			if err != nil {
				writeErr(w, http.StatusBadRequest, err)
				return
			}
			_ = h.auditSvc.Log("test", resource, strings.TrimSuffix(id, "/test"), r.Header.Get("X-Request-Id"), result.Status)
			writeJSON(w, http.StatusOK, result)
			return
		}
		switch r.Method {
		case http.MethodGet:
			item, err := h.platformSvc.Get(resource, id)
			if err != nil {
				writeErr(w, http.StatusBadRequest, err)
				return
			}
			writeJSON(w, http.StatusOK, item)
		case http.MethodPatch:
			var in domain.PlatformResource
			if !decodeBody(w, r, &in) {
				return
			}
			updated, err := h.platformSvc.Update(resource, id, in)
			if err != nil {
				writeErr(w, http.StatusBadRequest, err)
				return
			}
			_ = h.auditSvc.Log("update", resource, updated.ID, r.Header.Get("X-Request-Id"), updated.Key)
			writeJSON(w, http.StatusOK, updated)
		case http.MethodDelete:
			if err := h.platformSvc.Delete(resource, id); err != nil {
				writeErr(w, http.StatusBadRequest, err)
				return
			}
			_ = h.auditSvc.Log("delete", resource, id, r.Header.Get("X-Request-Id"), "")
			w.WriteHeader(http.StatusNoContent)
		default:
			methodNotAllowed(w)
		}
	}
}

func (h *HTTPHandler) handleValidateModel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var in domain.ValidateModelInput
	if !decodeBody(w, r, &in) {
		return
	}
	result, err := h.platformSvc.ValidateModel(in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *HTTPHandler) handleInspectProviderModel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var in domain.InspectProviderModelInput
	if !decodeBody(w, r, &in) {
		return
	}
	result, err := h.platformSvc.InspectProviderModel(in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
