package categories

import (
	"bullet-commerce/internal/models"
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestMockCategoryRepository_AllMethods(t *testing.T) {
	m := &MockCategoryRepository{}
	id := uuid.New()
	cat := &models.Category{ID: id, Name: "Go"}

	m.On("Create", mock.Anything, cat).Return(cat, nil)
	m.On("FindByID", mock.Anything, id).Return(cat, nil)
	m.On("FindAll", mock.Anything).Return([]models.Category{*cat}, nil)
	m.On("Update", mock.Anything, id, cat).Return(cat, nil)
	m.On("Delete", mock.Anything, id).Return(nil)
	m.On("FindByName", mock.Anything, "Go").Return(cat, nil)

	ctx := context.Background()
	m.Create(ctx, cat)
	m.FindByID(ctx, id)
	m.FindAll(ctx)
	m.Update(ctx, id, cat)
	m.Delete(ctx, id)
	m.FindByName(ctx, "Go")

	m.AssertExpectations(t)
}

func TestNewPostgresCategoryRepository(t *testing.T) {
	repo := NewPostgresCategoryRepository(nil)
	assert.NotNil(t, repo)
}
