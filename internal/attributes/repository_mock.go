package attributes

import (
	"bullet-commerce/internal/models"
	"context"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
)

type MockAttributeRepository struct {
	mock.Mock
}

func (m *MockAttributeRepository) FindByCode(ctx context.Context, code string) (*models.Attribute, error) {
	ret := m.Called(ctx, code)
	if ret.Get(0) == nil {
		return nil, ret.Error(1)
	}
	return ret.Get(0).(*models.Attribute), ret.Error(1)
}

func (m *MockAttributeRepository) ListValues(ctx context.Context, attributeID uuid.UUID) ([]models.AttributeValue, error) {
	ret := m.Called(ctx, attributeID)
	if ret.Get(0) == nil {
		return nil, ret.Error(1)
	}
	return ret.Get(0).([]models.AttributeValue), ret.Error(1)
}

func (m *MockAttributeRepository) LinkVariant(ctx context.Context, variantID, attributeValueID uuid.UUID) error {
	return m.Called(ctx, variantID, attributeValueID).Error(0)
}

func (m *MockAttributeRepository) ValuesForVariant(ctx context.Context, variantID uuid.UUID) ([]models.AttributeValue, error) {
	ret := m.Called(ctx, variantID)
	if ret.Get(0) == nil {
		return nil, ret.Error(1)
	}
	return ret.Get(0).([]models.AttributeValue), ret.Error(1)
}

func (m *MockAttributeRepository) FacetCounts(ctx context.Context, productID uuid.UUID) ([]models.AttributeFacet, error) {
	ret := m.Called(ctx, productID)
	if ret.Get(0) == nil {
		return nil, ret.Error(1)
	}
	return ret.Get(0).([]models.AttributeFacet), ret.Error(1)
}
