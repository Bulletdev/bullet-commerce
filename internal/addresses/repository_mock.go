package addresses

import (
	"bullet-commerce/internal/models"
	"context"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
)

// MockAddressRepository is a mock type for the AddressRepository interface
type MockAddressRepository struct {
	mock.Mock
}

// Create provides a mock function with given fields: ctx, address
func (_m *MockAddressRepository) Create(ctx context.Context, address *models.Address) (*models.Address, error) {
	ret := _m.Called(ctx, address)

	var r0 *models.Address
	if rf, ok := ret.Get(0).(func(context.Context, *models.Address) *models.Address); ok {
		r0 = rf(ctx, address)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*models.Address)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, *models.Address) error); ok {
		r1 = rf(ctx, address)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// FindByUserID provides a mock function with given fields: ctx, userID
func (_m *MockAddressRepository) FindByUserID(ctx context.Context, userID uuid.UUID) ([]models.Address, error) {
	ret := _m.Called(ctx, userID)

	var r0 []models.Address
	if rf, ok := ret.Get(0).(func(context.Context, uuid.UUID) []models.Address); ok {
		r0 = rf(ctx, userID)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]models.Address)
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

// FindByUserAndID provides a mock function with given fields: ctx, userID, addressID
func (_m *MockAddressRepository) FindByUserAndID(ctx context.Context, userID, addressID uuid.UUID) (*models.Address, error) {
	ret := _m.Called(ctx, userID, addressID)

	var r0 *models.Address
	if rf, ok := ret.Get(0).(func(context.Context, uuid.UUID, uuid.UUID) *models.Address); ok {
		r0 = rf(ctx, userID, addressID)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*models.Address)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, uuid.UUID, uuid.UUID) error); ok {
		r1 = rf(ctx, userID, addressID)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Update provides a mock function with given fields: ctx, userID, addressID, address
func (_m *MockAddressRepository) Update(ctx context.Context, userID, addressID uuid.UUID, address *models.Address) (*models.Address, error) {
	ret := _m.Called(ctx, userID, addressID, address)

	var r0 *models.Address
	if rf, ok := ret.Get(0).(func(context.Context, uuid.UUID, uuid.UUID, *models.Address) *models.Address); ok {
		r0 = rf(ctx, userID, addressID, address)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*models.Address)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, uuid.UUID, uuid.UUID, *models.Address) error); ok {
		r1 = rf(ctx, userID, addressID, address)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Delete provides a mock function with given fields: ctx, userID, addressID
func (_m *MockAddressRepository) Delete(ctx context.Context, userID, addressID uuid.UUID) error {
	ret := _m.Called(ctx, userID, addressID)

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, uuid.UUID, uuid.UUID) error); ok {
		r0 = rf(ctx, userID, addressID)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// SetDefault provides a mock function with given fields: ctx, userID, addressID
func (_m *MockAddressRepository) SetDefault(ctx context.Context, userID, addressID uuid.UUID) error {
	ret := _m.Called(ctx, userID, addressID)

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, uuid.UUID, uuid.UUID) error); ok {
		r0 = rf(ctx, userID, addressID)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// SetDefaultBilling provides a mock function with given fields: ctx, userID, addressID
func (_m *MockAddressRepository) SetDefaultBilling(ctx context.Context, userID, addressID uuid.UUID) error {
	ret := _m.Called(ctx, userID, addressID)

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, uuid.UUID, uuid.UUID) error); ok {
		r0 = rf(ctx, userID, addressID)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// SetDefaultShipping provides a mock function with given fields: ctx, userID, addressID
func (_m *MockAddressRepository) SetDefaultShipping(ctx context.Context, userID, addressID uuid.UUID) error {
	ret := _m.Called(ctx, userID, addressID)

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context, uuid.UUID, uuid.UUID) error); ok {
		r0 = rf(ctx, userID, addressID)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}
