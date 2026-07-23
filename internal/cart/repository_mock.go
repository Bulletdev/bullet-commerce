package cart

import (
	"bullet-commerce/internal/models"
	"context"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
)

// MockCartRepository is a mock type for the CartRepository type
type MockCartRepository struct {
	mock.Mock
}

// GetOrCreateCartByUserID mocks base method
func (_m *MockCartRepository) GetOrCreateCartByUserID(ctx context.Context, userID uuid.UUID) (*models.Cart, error) {
	ret := _m.Called(ctx, userID)

	var r0 *models.Cart
	if rf, ok := ret.Get(0).(func(context.Context, uuid.UUID) *models.Cart); ok {
		r0 = rf(ctx, userID)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*models.Cart)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, uuid.UUID) error); ok {
		r1 = rf(ctx, userID)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// GetCartItems mocks base method
func (_m *MockCartRepository) GetCartItems(ctx context.Context, cartID uuid.UUID) ([]models.CartItem, error) {
	ret := _m.Called(ctx, cartID)

	var r0 []models.CartItem
	if rf, ok := ret.Get(0).(func(context.Context, uuid.UUID) []models.CartItem); ok {
		r0 = rf(ctx, cartID)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]models.CartItem)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, uuid.UUID) error); ok {
		r1 = rf(ctx, cartID)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// AddItem mocks base method
func (_m *MockCartRepository) AddItem(ctx context.Context, cartID uuid.UUID, productID uuid.UUID, variantID uuid.UUID, quantity int, priceCents int64) (*models.CartItem, error) {
	ret := _m.Called(ctx, cartID, productID, variantID, quantity, priceCents)

	var r0 *models.CartItem
	if rf, ok := ret.Get(0).(func(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, int, int64) *models.CartItem); ok {
		r0 = rf(ctx, cartID, productID, variantID, quantity, priceCents)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*models.CartItem)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, int, int64) error); ok {
		r1 = rf(ctx, cartID, productID, variantID, quantity, priceCents)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// UpdateItemQuantity mocks base method
func (_m *MockCartRepository) UpdateItemQuantity(ctx context.Context, cartID, productID uuid.UUID, quantity int) (*models.CartItem, error) {
	ret := _m.Called(ctx, cartID, productID, quantity)

	var r0 *models.CartItem
	if rf, ok := ret.Get(0).(func(context.Context, uuid.UUID, uuid.UUID, int) *models.CartItem); ok {
		r0 = rf(ctx, cartID, productID, quantity)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*models.CartItem)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, uuid.UUID, uuid.UUID, int) error); ok {
		r1 = rf(ctx, cartID, productID, quantity)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// UpdateItem mocks base method - KEEPING OLD WRONG ONE FOR NOW TO AVOID BREAKING OLD TESTS THAT MIGHT STILL USE IT
// DELETE THIS OLD ONE LATER
func (_m *MockCartRepository) UpdateItem(ctx context.Context, cartID uuid.UUID, productID uuid.UUID, quantity int) (*models.CartItem, error) {
	ret := _m.Called(ctx, cartID, productID, quantity)

	var r0 *models.CartItem
	if rf, ok := ret.Get(0).(func(context.Context, uuid.UUID, uuid.UUID, int) *models.CartItem); ok {
		r0 = rf(ctx, cartID, productID, quantity)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*models.CartItem)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, uuid.UUID, uuid.UUID, int) error); ok {
		r1 = rf(ctx, cartID, productID, quantity)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// DeleteItem mocks base method
func (_m *MockCartRepository) DeleteItem(ctx context.Context, cartID uuid.UUID, productID uuid.UUID) error {
	ret := _m.Called(ctx, cartID, productID)

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, uuid.UUID, uuid.UUID) error); ok {
		r0 = rf(ctx, cartID, productID)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// RemoveItem mocks base method
func (_m *MockCartRepository) RemoveItem(ctx context.Context, cartID, productID uuid.UUID) error {
	ret := _m.Called(ctx, cartID, productID)

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, uuid.UUID, uuid.UUID) error); ok {
		r0 = rf(ctx, cartID, productID)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// ClearCart mocks base method
func (_m *MockCartRepository) ClearCart(ctx context.Context, cartID uuid.UUID) error {
	ret := _m.Called(ctx, cartID)

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, uuid.UUID) error); ok {
		r0 = rf(ctx, cartID)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// AddCouponCode mocks base method
func (_m *MockCartRepository) AddCouponCode(ctx context.Context, cartID uuid.UUID, code string) (*models.Cart, error) {
	ret := _m.Called(ctx, cartID, code)
	if ret.Get(0) == nil {
		return nil, ret.Error(1)
	}
	return ret.Get(0).(*models.Cart), ret.Error(1)
}

// RemoveCouponCode mocks base method
func (_m *MockCartRepository) RemoveCouponCode(ctx context.Context, cartID uuid.UUID, code string) (*models.Cart, error) {
	ret := _m.Called(ctx, cartID, code)
	if ret.Get(0) == nil {
		return nil, ret.Error(1)
	}
	return ret.Get(0).(*models.Cart), ret.Error(1)
}

// FindCartItem mocks base method
func (_m *MockCartRepository) FindCartItem(ctx context.Context, cartID uuid.UUID, productID uuid.UUID) (*models.CartItem, error) {
	ret := _m.Called(ctx, cartID, productID)

	var r0 *models.CartItem
	if rf, ok := ret.Get(0).(func(context.Context, uuid.UUID, uuid.UUID) *models.CartItem); ok {
		r0 = rf(ctx, cartID, productID)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*models.CartItem)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, uuid.UUID, uuid.UUID) error); ok {
		r1 = rf(ctx, cartID, productID)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}
