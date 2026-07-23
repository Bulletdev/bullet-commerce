package media

import (
	"bullet-commerce/internal/models"
	"context"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
)

type MockMediaRepository struct {
	mock.Mock
}

func (m *MockMediaRepository) Create(ctx context.Context, media *models.ProductMedia) (*models.ProductMedia, error) {
	ret := m.Called(ctx, media)
	if ret.Get(0) == nil {
		return nil, ret.Error(1)
	}
	return ret.Get(0).(*models.ProductMedia), ret.Error(1)
}

func (m *MockMediaRepository) ListByProduct(ctx context.Context, productID uuid.UUID) ([]models.ProductMedia, error) {
	ret := m.Called(ctx, productID)
	if ret.Get(0) == nil {
		return nil, ret.Error(1)
	}
	return ret.Get(0).([]models.ProductMedia), ret.Error(1)
}

func (m *MockMediaRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return m.Called(ctx, id).Error(0)
}
