package variants

import (
	"bullet-commerce/internal/models"
	"context"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
)

type MockVariantRepository struct {
	mock.Mock
}

func (m *MockVariantRepository) Create(ctx context.Context, variant *models.ProductVariant) (*models.ProductVariant, error) {
	ret := m.Called(ctx, variant)
	if ret.Get(0) == nil {
		return nil, ret.Error(1)
	}
	return ret.Get(0).(*models.ProductVariant), ret.Error(1)
}

func (m *MockVariantRepository) FindByID(ctx context.Context, id uuid.UUID) (*models.ProductVariant, error) {
	ret := m.Called(ctx, id)
	if ret.Get(0) == nil {
		return nil, ret.Error(1)
	}
	return ret.Get(0).(*models.ProductVariant), ret.Error(1)
}

func (m *MockVariantRepository) FindPublishedByID(ctx context.Context, id uuid.UUID) (*models.ProductVariant, error) {
	ret := m.Called(ctx, id)
	if ret.Get(0) == nil {
		return nil, ret.Error(1)
	}
	return ret.Get(0).(*models.ProductVariant), ret.Error(1)
}

func (m *MockVariantRepository) FindByProductID(ctx context.Context, productID uuid.UUID) ([]models.ProductVariant, error) {
	ret := m.Called(ctx, productID)
	if ret.Get(0) == nil {
		return nil, ret.Error(1)
	}
	return ret.Get(0).([]models.ProductVariant), ret.Error(1)
}

func (m *MockVariantRepository) SetStock(ctx context.Context, variantID, sourceID uuid.UUID, stock int) (int, int, error) {
	ret := m.Called(ctx, variantID, sourceID, stock)
	return ret.Int(0), ret.Int(1), ret.Error(2)
}

func (m *MockVariantRepository) Reserve(ctx context.Context, exec DBExecutor, variantID, sourceID uuid.UUID, qty int) error {
	return m.Called(ctx, exec, variantID, sourceID, qty).Error(0)
}

func (m *MockVariantRepository) Release(ctx context.Context, exec DBExecutor, variantID, sourceID uuid.UUID, qty int) error {
	return m.Called(ctx, exec, variantID, sourceID, qty).Error(0)
}

func (m *MockVariantRepository) Claim(ctx context.Context, exec DBExecutor, variantID, sourceID uuid.UUID, qty int) error {
	return m.Called(ctx, exec, variantID, sourceID, qty).Error(0)
}

func (m *MockVariantRepository) Restock(ctx context.Context, exec DBExecutor, variantID, sourceID uuid.UUID, qty int) error {
	return m.Called(ctx, exec, variantID, sourceID, qty).Error(0)
}

func (m *MockVariantRepository) AvailableForVariant(ctx context.Context, variantID uuid.UUID) (int, error) {
	ret := m.Called(ctx, variantID)
	return ret.Int(0), ret.Error(1)
}
