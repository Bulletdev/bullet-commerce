package products

import (
	"bullet-commerce/internal/models"
	"context"
	"regexp"
	"testing"

	"github.com/google/uuid"
	pgxmock "github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newBundleMock(t *testing.T) (pgxmock.PgxPoolIface, *postgresBundleRepository) {
	t.Helper()
	db, err := pgxmock.NewPool()
	require.NoError(t, err)
	return db, &postgresBundleRepository{db: db}
}

func TestCreateChoice_PersistsConstraints(t *testing.T) {
	db, repo := newBundleMock(t)
	productID := uuid.New()
	choiceID := uuid.New()

	// min/max/required must be handed to the INSERT so the slot constraints persist.
	db.ExpectQuery(regexp.QuoteMeta("INSERT INTO product_bundle_choices")).
		WithArgs(productID, 1, 3, true).
		WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(choiceID))

	c, err := repo.CreateChoice(context.Background(), &models.BundleChoice{
		ProductID: productID,
		MinQty:    1,
		MaxQty:    3,
		Required:  true,
	})
	require.NoError(t, err)
	assert.Equal(t, choiceID, c.ID)
	assert.Equal(t, 1, c.MinQty)
	assert.Equal(t, 3, c.MaxQty)
	assert.True(t, c.Required)
	assert.NoError(t, db.ExpectationsWereMet())
}

func TestListChoices_ReturnsRows(t *testing.T) {
	db, repo := newBundleMock(t)
	productID := uuid.New()
	choiceID := uuid.New()

	db.ExpectQuery(regexp.QuoteMeta("FROM product_bundle_choices")).
		WithArgs(productID).
		WillReturnRows(pgxmock.NewRows([]string{"id", "product_id", "min_qty", "max_qty", "required"}).
			AddRow(choiceID, productID, 1, 2, false))

	list, err := repo.ListChoices(context.Background(), productID)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, 1, list[0].MinQty)
	assert.Equal(t, 2, list[0].MaxQty)
	assert.False(t, list[0].Required)
}

func TestCreateOption_And_ListOptions(t *testing.T) {
	db, repo := newBundleMock(t)
	choiceID := uuid.New()
	optionID := uuid.New()
	optProductID := uuid.New()

	db.ExpectQuery(regexp.QuoteMeta("INSERT INTO product_bundle_options")).
		WithArgs(choiceID, optProductID, 1).
		WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow(optionID))

	o, err := repo.CreateOption(context.Background(), &models.BundleOption{
		ChoiceID:        choiceID,
		OptionProductID: optProductID,
		DefaultQty:      1,
	})
	require.NoError(t, err)
	assert.Equal(t, optionID, o.ID)

	db.ExpectQuery(regexp.QuoteMeta("FROM product_bundle_options")).
		WithArgs(choiceID).
		WillReturnRows(pgxmock.NewRows([]string{"id", "choice_id", "option_product_id", "default_qty"}).
			AddRow(optionID, choiceID, optProductID, 1))

	list, err := repo.ListOptions(context.Background(), choiceID)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, optProductID, list[0].OptionProductID)
	assert.NoError(t, db.ExpectationsWereMet())
}

func TestNewPostgresBundleRepository(t *testing.T) {
	assert.NotNil(t, NewPostgresBundleRepository(nil))
}
