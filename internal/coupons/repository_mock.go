package coupons

import (
	"bullet-commerce/internal/models"
	"context"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
)

type MockCouponRepository struct {
	mock.Mock
}

func (m *MockCouponRepository) FindByCode(ctx context.Context, code string) (*models.Coupon, error) {
	ret := m.Called(ctx, code)
	if ret.Get(0) == nil {
		return nil, ret.Error(1)
	}
	return ret.Get(0).(*models.Coupon), ret.Error(1)
}

func (m *MockCouponRepository) IncrementUse(ctx context.Context, id uuid.UUID) error {
	return m.Called(ctx, id).Error(0)
}
