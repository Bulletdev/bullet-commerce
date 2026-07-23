package addresses

import (
	"bullet-commerce/internal/models"
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestMockAddressRepository_AllMethods(t *testing.T) {
	m := &MockAddressRepository{}
	userID := uuid.New()
	addrID := uuid.New()
	addr := &models.Address{ID: addrID, UserID: userID}

	m.On("Create", mock.Anything, addr).Return(addr, nil)
	m.On("FindByUserID", mock.Anything, userID).Return([]models.Address{*addr}, nil)
	m.On("FindByUserAndID", mock.Anything, userID, addrID).Return(addr, nil)
	m.On("Update", mock.Anything, userID, addrID, addr).Return(addr, nil)
	m.On("Delete", mock.Anything, userID, addrID).Return(nil)
	m.On("SetDefault", mock.Anything, userID, addrID).Return(nil)

	ctx := context.Background()
	m.Create(ctx, addr)
	m.FindByUserID(ctx, userID)
	m.FindByUserAndID(ctx, userID, addrID)
	m.Update(ctx, userID, addrID, addr)
	m.Delete(ctx, userID, addrID)
	m.SetDefault(ctx, userID, addrID)

	m.AssertExpectations(t)
}

func TestNewPostgresAddressRepository(t *testing.T) {
	repo := NewPostgresAddressRepository(nil)
	assert.NotNil(t, repo)
}
