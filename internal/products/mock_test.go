package products

import (
	"bullet-commerce/internal/models"
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestMockProductRepository_AllMethods(t *testing.T) {
	m := &MockProductRepository{}
	id := uuid.New()
	catID := uuid.New()
	p := &models.Product{ID: id, Name: "X", PriceCents: 1}

	m.On("Create", mock.Anything, p).Return(p, nil)
	m.On("FindByID", mock.Anything, id).Return(p, nil)
	m.On("FindAll", mock.Anything, 20, 0).Return([]models.Product{*p}, nil)
	m.On("FindFeatured", mock.Anything).Return([]models.Product{*p}, nil)
	m.On("FindByCategoryID", mock.Anything, catID, 20, 0).Return([]models.Product{*p}, nil)
	m.On("Search", mock.Anything, "q", 20, 0).Return([]models.Product{*p}, nil)
	m.On("Update", mock.Anything, id, p).Return(p, nil)
	m.On("Delete", mock.Anything, id).Return(nil)
	m.On("UpdateStock", mock.Anything, id, 10).Return(nil)
	m.On("SetCategories", mock.Anything, id, []uuid.UUID{catID}).Return(nil)
	m.On("FindCategoryIDs", mock.Anything, id).Return([]uuid.UUID{catID}, nil)

	ctx := context.Background()
	m.Create(ctx, p)
	m.FindByID(ctx, id)
	m.FindAll(ctx, 20, 0)
	m.FindFeatured(ctx)
	m.FindByCategoryID(ctx, catID, 20, 0)
	m.Search(ctx, "q", 20, 0)
	m.Update(ctx, id, p)
	m.Delete(ctx, id)
	m.UpdateStock(ctx, id, 10)
	m.SetCategories(ctx, id, []uuid.UUID{catID})
	m.FindCategoryIDs(ctx, id)

	m.AssertExpectations(t)
}

func TestNewPostgresProductRepository(t *testing.T) {
	repo := NewPostgresProductRepository(nil)
	assert.NotNil(t, repo)
}
