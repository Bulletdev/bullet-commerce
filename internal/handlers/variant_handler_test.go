package handlers_test

import (
	"bullet-commerce/internal/handlers"
	"bullet-commerce/internal/media"
	"bullet-commerce/internal/models"
	"bullet-commerce/internal/variants"
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

func newVariantRouter(ph *handlers.ProductHandler) *mux.Router {
	r := mux.NewRouter()
	r.HandleFunc("/api/products/{id:[0-9a-fA-F-]+}", ph.GetProduct).Methods(http.MethodGet)
	r.HandleFunc("/api/products/{id:[0-9a-fA-F-]+}/variants", ph.CreateVariant).Methods(http.MethodPost)
	r.HandleFunc("/api/products/{id:[0-9a-fA-F-]+}/variants/{variantId:[0-9a-fA-F-]+}/stock", ph.UpdateVariantStock).Methods(http.MethodPatch)
	return r
}

// GetProduct exposes the product's variants alongside the (top-level) product fields.
func TestProductHandler_GetProduct_IncludesVariants(t *testing.T) {
	productRepo := new(MockProductRepository)
	variantRepo := new(variants.MockVariantRepository)
	mediaRepo := new(media.MockMediaRepository)
	ph := handlers.NewProductHandler(productRepo, variantRepo, mediaRepo, new(MockSourceRepository))

	productID, variantID := uuid.New(), uuid.New()
	productRepo.On("FindByID", mock.Anything, productID).Return(&models.Product{ID: productID, Name: "Tee", PriceCents: 5000}, nil).Once()
	variantRepo.On("FindByProductID", mock.Anything, productID).
		Return([]models.ProductVariant{{ID: variantID, ProductID: productID, SKU: "tee-m"}}, nil).Once()
	mediaRepo.On("ListByProduct", mock.Anything, productID).Return([]models.ProductMedia{}, nil).Once()
	productRepo.On("FindCategoryIDs", mock.Anything, productID).Return([]uuid.UUID{}, nil).Once()

	req := httptest.NewRequest(http.MethodGet, "/api/products/"+productID.String(), nil)
	rr := httptest.NewRecorder()
	newVariantRouter(ph).ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), `"name":"Tee"`)
	assert.Contains(t, rr.Body.String(), `"variants":[`)
	assert.Contains(t, rr.Body.String(), variantID.String())
	productRepo.AssertExpectations(t)
	variantRepo.AssertExpectations(t)
}

func TestProductHandler_CreateVariant_Success(t *testing.T) {
	productRepo := new(MockProductRepository)
	variantRepo := new(variants.MockVariantRepository)
	ph := handlers.NewProductHandler(productRepo, variantRepo, new(media.MockMediaRepository), new(MockSourceRepository))

	productID, variantID := uuid.New(), uuid.New()
	productRepo.On("FindByIDAdmin", mock.Anything, productID).Return(&models.Product{ID: productID}, nil).Once()
	variantRepo.On("Create", mock.Anything, mock.MatchedBy(func(v *models.ProductVariant) bool {
		return v.ProductID == productID && v.SKU == "tee-m" && v.Stock == 7
	})).Return(&models.ProductVariant{ID: variantID, ProductID: productID, SKU: "tee-m", Stock: 7}, nil).Once()

	body := `{"sku":"tee-m","stock":7,"attributes":{"size":"M"}}`
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/products/%s/variants", productID), strings.NewReader(body))
	rr := httptest.NewRecorder()
	newVariantRouter(ph).ServeHTTP(rr, req)

	require.Equal(t, http.StatusCreated, rr.Code)
	assert.Contains(t, rr.Body.String(), variantID.String())
	productRepo.AssertExpectations(t)
	variantRepo.AssertExpectations(t)
}

func TestProductHandler_CreateVariant_MissingSKU(t *testing.T) {
	productRepo := new(MockProductRepository)
	variantRepo := new(variants.MockVariantRepository)
	ph := handlers.NewProductHandler(productRepo, variantRepo, new(media.MockMediaRepository), new(MockSourceRepository))

	productID := uuid.New()
	productRepo.On("FindByIDAdmin", mock.Anything, productID).Return(&models.Product{ID: productID}, nil).Once()

	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/products/%s/variants", productID), strings.NewReader(`{"stock":1}`))
	rr := httptest.NewRecorder()
	newVariantRouter(ph).ServeHTTP(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "sku is required")
}

func TestProductHandler_UpdateVariantStock_Success(t *testing.T) {
	productRepo := new(MockProductRepository)
	variantRepo := new(variants.MockVariantRepository)
	sourceRepo := new(MockSourceRepository)
	ph := handlers.NewProductHandler(productRepo, variantRepo, new(media.MockMediaRepository), sourceRepo)

	productID, variantID, sourceID := uuid.New(), uuid.New(), uuid.New()
	// No source_id in the body -> default source; absolute UPSERT; aggregate read for the response.
	variantRepo.On("FindByID", mock.Anything, variantID).Return(&models.ProductVariant{ID: variantID}, nil).Once()
	sourceRepo.On("GetDefault", mock.Anything).Return(&models.Source{ID: sourceID, IsDefault: true}, nil).Once()
	variantRepo.On("SetStock", mock.Anything, variantID, sourceID, 42).Return(42, 0, nil).Once()
	variantRepo.On("AvailableForVariant", mock.Anything, variantID).Return(42, nil).Once()

	req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/products/%s/variants/%s/stock", productID, variantID), strings.NewReader(`{"stock":42}`))
	rr := httptest.NewRecorder()
	newVariantRouter(ph).ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), `"stock_available":42`)
	assert.Contains(t, rr.Body.String(), variantID.String())
	assert.Contains(t, rr.Body.String(), sourceID.String())
	variantRepo.AssertExpectations(t)
	sourceRepo.AssertExpectations(t)
}

func TestProductHandler_UpdateVariantStock_NotFound(t *testing.T) {
	productRepo := new(MockProductRepository)
	variantRepo := new(variants.MockVariantRepository)
	sourceRepo := new(MockSourceRepository)
	ph := handlers.NewProductHandler(productRepo, variantRepo, new(media.MockMediaRepository), sourceRepo)

	productID, variantID := uuid.New(), uuid.New()
	// Unknown variant is rejected before any stock write.
	variantRepo.On("FindByID", mock.Anything, variantID).Return(nil, variants.ErrVariantNotFound).Once()

	req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/products/%s/variants/%s/stock", productID, variantID), strings.NewReader(`{"stock":5}`))
	rr := httptest.NewRecorder()
	newVariantRouter(ph).ServeHTTP(rr, req)

	require.Equal(t, http.StatusNotFound, rr.Code)
	variantRepo.AssertExpectations(t)
}

func TestProductHandler_UpdateVariantStock_BelowReserved(t *testing.T) {
	productRepo := new(MockProductRepository)
	variantRepo := new(variants.MockVariantRepository)
	sourceRepo := new(MockSourceRepository)
	ph := handlers.NewProductHandler(productRepo, variantRepo, new(media.MockMediaRepository), sourceRepo)

	productID, variantID, sourceID := uuid.New(), uuid.New(), uuid.New()
	variantRepo.On("FindByID", mock.Anything, variantID).Return(&models.ProductVariant{ID: variantID}, nil).Once()
	sourceRepo.On("GetDefault", mock.Anything).Return(&models.Source{ID: sourceID, IsDefault: true}, nil).Once()
	// Setting below the reserved units is refused with 409.
	variantRepo.On("SetStock", mock.Anything, variantID, sourceID, 1).Return(0, 0, &variants.StockBelowReservedError{Reserved: 3}).Once()

	req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/products/%s/variants/%s/stock", productID, variantID), strings.NewReader(`{"stock":1}`))
	rr := httptest.NewRecorder()
	newVariantRouter(ph).ServeHTTP(rr, req)

	require.Equal(t, http.StatusConflict, rr.Code)
	assert.Contains(t, rr.Body.String(), "reserved")
	variantRepo.AssertExpectations(t)
	sourceRepo.AssertExpectations(t)
}
