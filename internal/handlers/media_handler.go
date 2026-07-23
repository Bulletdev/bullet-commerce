package handlers

import (
	"bullet-commerce/internal/media"
	"bullet-commerce/internal/models"
	"bullet-commerce/internal/storage"
	"bullet-commerce/internal/webutils"
	"errors"
	"net/http"
	"path"
	"strings"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

// MediaHandler serves the two product-media ingestion flows plus deletion. Storage may be nil:
// when object storage is not configured the presign endpoint answers 501 and only the
// URL-reference flow (AddMedia) stays available - the catalog can still carry CDN images.
type MediaHandler struct {
	MediaRepo media.MediaRepository
	Storage   storage.Provider
}

func NewMediaHandler(mediaRepo media.MediaRepository, store storage.Provider) *MediaHandler {
	return &MediaHandler{MediaRepo: mediaRepo, Storage: store}
}

// AddMediaRequest is the body of POST /api/products/{id}/media - registering media by URL,
// whether that URL is an existing CDN object or the public URL returned by a presigned upload.
type AddMediaRequest struct {
	URL       string     `json:"url"`
	Alt       *string    `json:"alt,omitempty"`
	Position  int        `json:"position,omitempty"`
	VariantID *uuid.UUID `json:"variant_id,omitempty"`
	Kind      string     `json:"kind,omitempty"`
}

// AddMedia handles POST /api/products/{id}/media (admin): register a media row by URL.
func (h *MediaHandler) AddMedia(w http.ResponseWriter, r *http.Request) {
	productID, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		webutils.ErrorJSON(w, errors.New("invalid product ID format"), http.StatusBadRequest)
		return
	}

	var req AddMediaRequest
	if err := webutils.ReadJSON(r, &req); err != nil {
		webutils.ErrorJSON(w, errors.New("invalid request body"), http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.URL) == "" {
		webutils.ErrorJSON(w, errors.New("url is required"), http.StatusBadRequest)
		return
	}
	if req.Position < 0 {
		webutils.ErrorJSON(w, errors.New("position cannot be negative"), http.StatusBadRequest)
		return
	}

	// Empty kind defaults to image; a supplied kind must be one the schema accepts, so the
	// 400 here matches the DB CHECK instead of letting the insert fail as a 500.
	kind := req.Kind
	if kind == "" {
		kind = models.MediaKindImage
	} else if !models.ValidMediaKind(kind) {
		webutils.ErrorJSON(w, errors.New("invalid media kind"), http.StatusBadRequest)
		return
	}

	created, err := h.MediaRepo.Create(r.Context(), &models.ProductMedia{
		ProductID: productID,
		VariantID: req.VariantID,
		URL:       req.URL,
		Alt:       req.Alt,
		Kind:      kind,
		Position:  req.Position,
	})
	if err != nil {
		webutils.ErrorJSON(w, errors.New("failed to create media"), http.StatusInternalServerError)
		return
	}

	webutils.WriteJSON(w, http.StatusCreated, created)
}

// UploadURLRequest is the body of POST /api/media/upload-url - asking for a presigned PUT.
type UploadURLRequest struct {
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
}

// UploadURLResponse is what the client needs to upload directly to the bucket and then register
// the result: PUT the bytes to UploadURL, then POST PublicURL back via AddMedia.
type UploadURLResponse struct {
	UploadURL string `json:"upload_url"`
	PublicURL string `json:"public_url"`
	Key       string `json:"key"`
}

// UploadURL handles POST /api/media/upload-url (admin): mint a presigned PUT URL. When object
// storage is not configured it answers 501 - the URL-reference flow (AddMedia) still works, so
// this is "feature not enabled", not a client error.
func (h *MediaHandler) UploadURL(w http.ResponseWriter, r *http.Request) {
	if h.Storage == nil {
		webutils.ErrorJSON(w, errors.New("object storage is not configured"), http.StatusNotImplemented)
		return
	}

	var req UploadURLRequest
	if err := webutils.ReadJSON(r, &req); err != nil {
		webutils.ErrorJSON(w, errors.New("invalid request body"), http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Filename) == "" {
		webutils.ErrorJSON(w, errors.New("filename is required"), http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.ContentType) == "" {
		webutils.ErrorJSON(w, errors.New("content_type is required"), http.StatusBadRequest)
		return
	}

	key := buildObjectKey(req.Filename)
	uploadURL, publicURL, err := h.Storage.PresignPut(r.Context(), key, req.ContentType)
	if err != nil {
		webutils.ErrorJSON(w, errors.New("failed to generate upload url"), http.StatusInternalServerError)
		return
	}

	webutils.WriteJSON(w, http.StatusOK, UploadURLResponse{
		UploadURL: uploadURL,
		PublicURL: publicURL,
		Key:       key,
	})
}

// DeleteMedia handles DELETE /api/media/{id} (admin).
func (h *MediaHandler) DeleteMedia(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(mux.Vars(r)["id"])
	if err != nil {
		webutils.ErrorJSON(w, errors.New("invalid media ID format"), http.StatusBadRequest)
		return
	}

	if err := h.MediaRepo.Delete(r.Context(), id); err != nil {
		if errors.Is(err, media.ErrMediaNotFound) {
			webutils.ErrorJSON(w, err, http.StatusNotFound)
		} else {
			webutils.ErrorJSON(w, errors.New("failed to delete media"), http.StatusInternalServerError)
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// buildObjectKey namespaces every upload under a random UUID so two files with the same name
// never collide, while preserving the original basename/extension for readability and correct
// content sniffing by CDNs. WHY strip the directory: a client-supplied path must not let the
// key escape the intended prefix.
func buildObjectKey(filename string) string {
	base := path.Base(filepathClean(filename))
	if base == "." || base == "/" || base == "" {
		base = "file"
	}
	return "products/media/" + uuid.NewString() + "/" + base
}

// filepathClean normalizes a client filename using slash semantics (URLs/keys are slash-based)
// without importing path/filepath, whose behavior is OS-dependent.
func filepathClean(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	return path.Clean("/" + name)
}
