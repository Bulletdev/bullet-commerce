package users

import (
	"bullet-commerce/internal/models"
	"context"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
)

// MockUserRepository is a mock type for the UserRepository interface
type MockUserRepository struct {
	mock.Mock
}

// Create provides a mock function with given fields: ctx, name, email, passwordHash
func (_m *MockUserRepository) Create(ctx context.Context, name string, email string, passwordHash string) (*models.User, error) {
	ret := _m.Called(ctx, name, email, passwordHash)

	var r0 *models.User
	if rf, ok := ret.Get(0).(func(context.Context, string, string, string) *models.User); ok {
		r0 = rf(ctx, name, email, passwordHash)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*models.User)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, string, string, string) error); ok {
		r1 = rf(ctx, name, email, passwordHash)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

func (_m *MockUserRepository) FindByEmail(ctx context.Context, email string) (*models.User, error) {
	ret := _m.Called(ctx, email)

	var r0 *models.User
	if rf, ok := ret.Get(0).(func(context.Context, string) *models.User); ok {
		r0 = rf(ctx, email)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*models.User)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, string) error); ok {
		r1 = rf(ctx, email)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

func (_m *MockUserRepository) FindByID(ctx context.Context, id uuid.UUID) (*models.User, error) {
	ret := _m.Called(ctx, id)

	var r0 *models.User
	if rf, ok := ret.Get(0).(func(context.Context, uuid.UUID) *models.User); ok {
		r0 = rf(ctx, id)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*models.User)
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
