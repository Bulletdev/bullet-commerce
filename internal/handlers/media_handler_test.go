package handlers_test

import (
	"bullet-commerce/internal/handlers"
	"bullet-commerce/internal/media"
	"bullet-commerce/internal/models"
	"bullet-commerce/internal/storage"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func newMediaRouter(mh *handlers.MediaHandler) *mux.Router {
	r := mux.NewRouter()
	r.HandleFunc("/api/products/{id:[0-9a-fA-F-]+}/media", mh.AddMedia).Methods(http.MethodPost)
	r.HandleFunc("/api/media/upload-url", mh.UploadURL).Methods(http.MethodPost)
	r.HandleFunc("/api/media/{id:[0-9a-fA-F-]+}", mh.DeleteMedia).Methods(http.MethodDelete)
	return r
}

func TestMediaHandler_AddMedia_Success(t *testing.T) {
	repo := new(media.MockMediaRepository)
	mh := handlers.NewMediaHandler(repo, nil)

	productID, mediaID := uuid.New(), uuid.New()
	repo.On("Create", mock.Anything, mock.MatchedBy(func(m *models.ProductMedia) bool {
		return m.ProductID == productID && m.URL == "https://cdn/x.jpg" && m.Kind == models.MediaKindImage
	})).Return(&models.ProductMedia{ID: mediaID, ProductID: productID, URL: "https://cdn/x.jpg", Kind: models.MediaKindImage}, nil).Once()

	body := `{"url":"https://cdn/x.jpg"}`
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/products/%s/media", productID), strings.NewReader(body))
	rr := httptest.NewRecorder()
	newMediaRouter(mh).ServeHTTP(rr, req)

	require.Equal(t, http.StatusCreated, rr.Code)
	assert.Contains(t, rr.Body.String(), mediaID.String())
	repo.AssertExpectations(t)
}

func TestMediaHandler_AddMedia_MissingURL(t *testing.T) {
	repo := new(media.MockMediaRepository)
	mh := handlers.NewMediaHandler(repo, nil)

	productID := uuid.New()
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/products/%s/media", productID), strings.NewReader(`{"alt":"x"}`))
	rr := httptest.NewRecorder()
	newMediaRouter(mh).ServeHTTP(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "url is required")
}

func TestMediaHandler_AddMedia_InvalidKind(t *testing.T) {
	repo := new(media.MockMediaRepository)
	mh := handlers.NewMediaHandler(repo, nil)

	productID := uuid.New()
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/products/%s/media", productID), strings.NewReader(`{"url":"https://cdn/x","kind":"audio"}`))
	rr := httptest.NewRecorder()
	newMediaRouter(mh).ServeHTTP(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "invalid media kind")
}

// With storage configured, the presign endpoint returns the upload URL, public URL and key.
func TestMediaHandler_UploadURL_Active(t *testing.T) {
	repo := new(media.MockMediaRepository)
	mh := handlers.NewMediaHandler(repo, &storage.FakeProvider{PublicBaseURL: "https://cdn.example.com"})

	body := `{"filename":"photo.jpg","content_type":"image/jpeg"}`
	req := httptest.NewRequest(http.MethodPost, "/api/media/upload-url", strings.NewReader(body))
	rr := httptest.NewRecorder()
	newMediaRouter(mh).ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), `"upload_url"`)
	assert.Contains(t, rr.Body.String(), `"public_url"`)
	assert.Contains(t, rr.Body.String(), "https://cdn.example.com/products/media/")
	assert.Contains(t, rr.Body.String(), "photo.jpg")
}

// With storage nil (not configured), the presign endpoint answers 501 - the URL-reference
// flow keeps working, so this is "feature disabled", not a client error.
func TestMediaHandler_UploadURL_Disabled(t *testing.T) {
	repo := new(media.MockMediaRepository)
	mh := handlers.NewMediaHandler(repo, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/media/upload-url", strings.NewReader(`{"filename":"x.jpg","content_type":"image/jpeg"}`))
	rr := httptest.NewRecorder()
	newMediaRouter(mh).ServeHTTP(rr, req)

	require.Equal(t, http.StatusNotImplemented, rr.Code)
}

func TestMediaHandler_UploadURL_MissingFilename(t *testing.T) {
	repo := new(media.MockMediaRepository)
	mh := handlers.NewMediaHandler(repo, &storage.FakeProvider{PublicBaseURL: "https://cdn.example.com"})

	req := httptest.NewRequest(http.MethodPost, "/api/media/upload-url", strings.NewReader(`{"content_type":"image/jpeg"}`))
	rr := httptest.NewRecorder()
	newMediaRouter(mh).ServeHTTP(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "filename is required")
}

func TestMediaHandler_DeleteMedia_Success(t *testing.T) {
	repo := new(media.MockMediaRepository)
	mh := handlers.NewMediaHandler(repo, nil)

	id := uuid.New()
	repo.On("Delete", mock.Anything, id).Return(nil).Once()

	req := httptest.NewRequest(http.MethodDelete, "/api/media/"+id.String(), nil)
	rr := httptest.NewRecorder()
	newMediaRouter(mh).ServeHTTP(rr, req)

	require.Equal(t, http.StatusNoContent, rr.Code)
	repo.AssertExpectations(t)
}

func TestMediaHandler_DeleteMedia_NotFound(t *testing.T) {
	repo := new(media.MockMediaRepository)
	mh := handlers.NewMediaHandler(repo, nil)

	id := uuid.New()
	repo.On("Delete", mock.Anything, id).Return(media.ErrMediaNotFound).Once()

	req := httptest.NewRequest(http.MethodDelete, "/api/media/"+id.String(), nil)
	rr := httptest.NewRecorder()
	newMediaRouter(mh).ServeHTTP(rr, req)

	require.Equal(t, http.StatusNotFound, rr.Code)
	repo.AssertExpectations(t)
}
