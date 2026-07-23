package cart

import (
	"bullet-commerce/internal/models"
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestMockCartRepository_AllMethods(t *testing.T) {
	m := &MockCartRepository{}
	userID := uuid.New()
	cartID := uuid.New()
	prodID := uuid.New()
	itemID := uuid.New()
	cart := &models.Cart{ID: cartID}
	item := &models.CartItem{ID: itemID}

	m.On("GetOrCreateCartByUserID", mock.Anything, userID).Return(cart, nil)
	variantID := uuid.New()
	m.On("GetCartItems", mock.Anything, cartID).Return([]models.CartItem{*item}, nil)
	m.On("AddItem", mock.Anything, cartID, prodID, variantID, 1, int64(10)).Return(item, nil)
	m.On("UpdateItemQuantity", mock.Anything, cartID, prodID, 2).Return(item, nil)
	m.On("UpdateItem", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(item, nil).Maybe()
	m.On("DeleteItem", mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	m.On("RemoveItem", mock.Anything, cartID, prodID).Return(nil)
	m.On("ClearCart", mock.Anything, cartID).Return(nil)
	m.On("FindCartItem", mock.Anything, cartID, prodID).Return(item, nil)

	ctx := context.Background()
	m.GetOrCreateCartByUserID(ctx, userID)
	m.GetCartItems(ctx, cartID)
	m.AddItem(ctx, cartID, prodID, variantID, 1, int64(10))
	m.UpdateItemQuantity(ctx, cartID, prodID, 2)
	m.RemoveItem(ctx, cartID, prodID)
	m.ClearCart(ctx, cartID)
	m.FindCartItem(ctx, cartID, prodID)

	m.AssertExpectations(t)
}

func TestNewPostgresCartRepository(t *testing.T) {
	repo := NewPostgresCartRepository(nil)
	assert.NotNil(t, repo)
}
