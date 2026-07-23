package products

import (
	"bullet-commerce/internal/models"
	"context"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
)

type MockProductRepository struct {
	mock.Mock
}

func (m *MockProductRepository) Create(ctx context.Context, product *models.Product) (*models.Product, error) {
	ret := m.Called(ctx, product)
	if ret.Get(0) == nil {
		return nil, ret.Error(1)
	}
	return ret.Get(0).(*models.Product), ret.Error(1)
}

func (m *MockProductRepository) FindByID(ctx context.Context, id uuid.UUID) (*models.Product, error) {
	ret := m.Called(ctx, id)
	if ret.Get(0) == nil {
		return nil, ret.Error(1)
	}
	return ret.Get(0).(*models.Product), ret.Error(1)
}

func (m *MockProductRepository) FindByIDAdmin(ctx context.Context, id uuid.UUID) (*models.Product, error) {
	ret := m.Called(ctx, id)
	if ret.Get(0) == nil {
		return nil, ret.Error(1)
	}
	return ret.Get(0).(*models.Product), ret.Error(1)
}

func (m *MockProductRepository) FindAll(ctx context.Context, limit, offset int) ([]models.Product, error) {
	ret := m.Called(ctx, limit, offset)
	if ret.Get(0) == nil {
		return nil, ret.Error(1)
	}
	return ret.Get(0).([]models.Product), ret.Error(1)
}

func (m *MockProductRepository) FindFeatured(ctx context.Context) ([]models.Product, error) {
	ret := m.Called(ctx)
	if ret.Get(0) == nil {
		return nil, ret.Error(1)
	}
	return ret.Get(0).([]models.Product), ret.Error(1)
}

func (m *MockProductRepository) FindByCategoryID(ctx context.Context, categoryID uuid.UUID, limit, offset int) ([]models.Product, error) {
	ret := m.Called(ctx, categoryID, limit, offset)
	if ret.Get(0) == nil {
		return nil, ret.Error(1)
	}
	return ret.Get(0).([]models.Product), ret.Error(1)
}

func (m *MockProductRepository) Search(ctx context.Context, query string, limit, offset int) ([]models.Product, error) {
	ret := m.Called(ctx, query, limit, offset)
	if ret.Get(0) == nil {
		return nil, ret.Error(1)
	}
	return ret.Get(0).([]models.Product), ret.Error(1)
}

func (m *MockProductRepository) Update(ctx context.Context, id uuid.UUID, product *models.Product) (*models.Product, error) {
	ret := m.Called(ctx, id, product)

	var r0 *models.Product
	if rf, ok := ret.Get(0).(func(context.Context, uuid.UUID, *models.Product) *models.Product); ok {
		r0 = rf(ctx, id, product)
	} else if ret.Get(0) != nil {
		r0 = ret.Get(0).(*models.Product)
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, uuid.UUID, *models.Product) error); ok {
		r1 = rf(ctx, id, product)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

func (m *MockProductRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return m.Called(ctx, id).Error(0)
}

func (m *MockProductRepository) UpdateStock(ctx context.Context, id uuid.UUID, stock int) error {
	return m.Called(ctx, id, stock).Error(0)
}

func (m *MockProductRepository) SetCategories(ctx context.Context, productID uuid.UUID, categoryIDs []uuid.UUID) error {
	return m.Called(ctx, productID, categoryIDs).Error(0)
}

func (m *MockProductRepository) FindCategoryIDs(ctx context.Context, productID uuid.UUID) ([]uuid.UUID, error) {
	ret := m.Called(ctx, productID)
	if ret.Get(0) == nil {
		return nil, ret.Error(1)
	}
	return ret.Get(0).([]uuid.UUID), ret.Error(1)
}
