package transport

import (
	"arenea/backend/internal/domain"
	"errors"
	"net/http"
	"strings"
)

func (h *HTTPHandler) handleAvatarAssets(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		q := r.URL.Query()
		items, err := h.platformSvc.ListAvatarAssets(q.Get("scope"), q.Get("workspace_id"), q.Get("owner_user_id"))
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, listResponse[domain.AvatarAsset]{Items: items})
	case http.MethodPost:
		if err := r.ParseMultipartForm(3 << 20); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		defer file.Close()
		created, err := h.platformSvc.UploadAvatar(file, header, r.FormValue("workspace_id"), r.FormValue("owner_user_id"))
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		_ = h.auditSvc.Log("create", "avatar-assets", created.ID, r.Header.Get("X-Request-Id"), created.Key)
		writeJSON(w, http.StatusCreated, created)
	default:
		methodNotAllowed(w)
	}
}

func (h *HTTPHandler) handleAvatarAssetByID(w http.ResponseWriter, r *http.Request) {
	path := idFromPath(r.URL.Path, "/api/v1/avatar-assets/")
	if path == "" {
		writeErr(w, http.StatusBadRequest, errors.New("avatar id is required"))
		return
	}
	parts := strings.Split(strings.Trim(path, "/"), "/")
	id := parts[0]
	if len(parts) >= 2 && (parts[1] == "file" || parts[1] == "thumbnail") {
		h.writeAvatarImage(w, r, id, parts[1] == "thumbnail")
		return
	}
	if r.Method == http.MethodDelete {
		if err := h.platformSvc.Delete("avatar-assets", id); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		_ = h.auditSvc.Log("delete", "avatar-assets", id, r.Header.Get("X-Request-Id"), "")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	methodNotAllowed(w)
}

func (h *HTTPHandler) writeAvatarImage(w http.ResponseWriter, r *http.Request, id string, thumbnail bool) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	img, err := h.platformSvc.GetAvatarImage(id, thumbnail)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	w.Header().Set("Content-Type", img.MimeType)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(img.Data)
}
