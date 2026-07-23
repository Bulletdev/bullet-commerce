package categories

import (
	"bullet-commerce/internal/models"
	"context"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
)

// MockCategoryRepository is a mock type for the CategoryRepository interface
type MockCategoryRepository struct {
	mock.Mock
}

// Create provides a mock function with given fields: ctx, category
func (_m *MockCategoryRepository) Create(ctx context.Context, category *models.Category) (*models.Category, error) {
	ret := _m.Called(ctx, category)

	var r0 *models.Category
	if rf, ok := ret.Get(0).(func(context.Context, *models.Category) *models.Category); ok {
		r0 = rf(ctx, category)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*models.Category)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, *models.Category) error); ok {
		r1 = rf(ctx, category)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// FindByID provides a mock function with given fields: ctx, id
func (_m *MockCategoryRepository) FindByID(ctx context.Context, id uuid.UUID) (*models.Category, error) {
	ret := _m.Called(ctx, id)

	var r0 *models.Category
	if rf, ok := ret.Get(0).(func(context.Context, uuid.UUID) *models.Category); ok {
		r0 = rf(ctx, id)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*models.Category)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, uuid.UUID) error); ok {
		r1 = rf(ctx, id)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// FindAll provides a mock function with given fields: ctx
func (_m *MockCategoryRepository) FindAll(ctx context.Context) ([]models.Category, error) {
	ret := _m.Called(ctx)

	var r0 []models.Category
	if rf, ok := ret.Get(0).(func(context.Context) []models.Category); ok {
		r0 = rf(ctx)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]models.Category)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context) error); ok {
		r1 = rf(ctx)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Update provides a mock function with given fields: ctx, id, category
func (_m *MockCategoryRepository) Update(ctx context.Context, id uuid.UUID, category *models.Category) (*models.Category, error) {
	ret := _m.Called(ctx, id, category)

	var r0 *models.Category
	if rf, ok := ret.Get(0).(func(context.Context, uuid.UUID, *models.Category) *models.Category); ok {
		r0 = rf(ctx, id, category)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*models.Category)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, uuid.UUID, *models.Category) error); ok {
		r1 = rf(ctx, id, category)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Delete provides a mock function with given fields: ctx, id
func (_m *MockCategoryRepository) Delete(ctx context.Context, id uuid.UUID) error {
	ret := _m.Called(ctx, id)

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, uuid.UUID) error); ok {
		r0 = rf(ctx, id)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// FindByName provides a mock function with given fields: ctx, name
func (_m *MockCategoryRepository) FindByName(ctx context.Context, name string) (*models.Category, error) {
	ret := _m.Called(ctx, name)

	var r0 *models.Category
	if rf, ok := ret.Get(0).(func(context.Context, string) *models.Category); ok {
		r0 = rf(ctx, name)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*models.Category)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, string) error); ok {
		r1 = rf(ctx, name)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}
