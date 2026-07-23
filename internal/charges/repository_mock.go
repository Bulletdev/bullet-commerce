package charges

import (
	"bullet-commerce/internal/models"
	"context"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
)

type MockChargeRepository struct {
	mock.Mock
}

func (m *MockChargeRepository) Create(ctx context.Context, charge *models.Charge) error {
	return m.Called(ctx, charge).Error(0)
}

func (m *MockChargeRepository) FindByOrderID(ctx context.Context, orderID uuid.UUID) ([]models.Charge, error) {
	ret := m.Called(ctx, orderID)
	if ret.Get(0) == nil {
		return nil, ret.Error(1)
	}
	return ret.Get(0).([]models.Charge), ret.Error(1)
}
