package users

import (
	"bullet-commerce/internal/models"
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestMockUserRepository_AllMethods(t *testing.T) {
	m := &MockUserRepository{}
	id := uuid.New()
	u := &models.User{ID: id}

	m.On("Create", mock.Anything, "n", "e", "h").Return(u, nil)
	m.On("FindByEmail", mock.Anything, "e").Return(u, nil)
	m.On("FindByID", mock.Anything, id).Return(u, nil)

	ctx := context.Background()
	m.Create(ctx, "n", "e", "h")
	m.FindByEmail(ctx, "e")
	m.FindByID(ctx, id)

	m.AssertExpectations(t)
}

func TestNewPostgresUserRepository(t *testing.T) {
	repo := NewPostgresUserRepository(nil)
	assert.NotNil(t, repo)
}
