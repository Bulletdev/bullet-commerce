package products

import (
	"bullet-commerce/internal/models"
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// BundleRepository persists a bundle product's composition (choices and their
// options). It is kept separate from ProductRepository because a bundle's structure
// is only relevant to bundle-type products, and most product reads never need it.
type BundleRepository interface {
	CreateChoice(ctx context.Context, choice *models.BundleChoice) (*models.BundleChoice, error)
	ListChoices(ctx context.Context, productID uuid.UUID) ([]models.BundleChoice, error)
	CreateOption(ctx context.Context, option *models.BundleOption) (*models.BundleOption, error)
	ListOptions(ctx context.Context, choiceID uuid.UUID) ([]models.BundleOption, error)
}

type postgresBundleRepository struct {
	db DBPool
}

func NewPostgresBundleRepository(db *pgxpool.Pool) BundleRepository {
	return &postgresBundleRepository{db: db}
}

func (r *postgresBundleRepository) CreateChoice(ctx context.Context, choice *models.BundleChoice) (*models.BundleChoice, error) {
	query := `
		INSERT INTO product_bundle_choices (product_id, min_qty, max_qty, required)
		VALUES ($1, $2, $3, $4)
		RETURNING id
	`
	err := r.db.QueryRow(ctx, query,
		choice.ProductID, choice.MinQty, choice.MaxQty, choice.Required,
	).Scan(&choice.ID)
	if err != nil {
		return nil, err
	}
	return choice, nil
}

func (r *postgresBundleRepository) ListChoices(ctx context.Context, productID uuid.UUID) ([]models.BundleChoice, error) {
	query := `
		SELECT id, product_id, min_qty, max_qty, required
		FROM product_bundle_choices
		WHERE product_id = $1
		ORDER BY created_at
	`
	rows, err := r.db.Query(ctx, query, productID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []models.BundleChoice
	for rows.Next() {
		var c models.BundleChoice
		if err := rows.Scan(&c.ID, &c.ProductID, &c.MinQty, &c.MaxQty, &c.Required); err != nil {
			return nil, err
		}
		list = append(list, c)
	}
	return list, rows.Err()
}

func (r *postgresBundleRepository) CreateOption(ctx context.Context, option *models.BundleOption) (*models.BundleOption, error) {
	query := `
		INSERT INTO product_bundle_options (choice_id, option_product_id, default_qty)
		VALUES ($1, $2, $3)
		RETURNING id
	`
	err := r.db.QueryRow(ctx, query,
		option.ChoiceID, option.OptionProductID, option.DefaultQty,
	).Scan(&option.ID)
	if err != nil {
		return nil, err
	}
	return option, nil
}

func (r *postgresBundleRepository) ListOptions(ctx context.Context, choiceID uuid.UUID) ([]models.BundleOption, error) {
	query := `
		SELECT id, choice_id, option_product_id, default_qty
		FROM product_bundle_options
		WHERE choice_id = $1
		ORDER BY created_at
	`
	rows, err := r.db.Query(ctx, query, choiceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []models.BundleOption
	for rows.Next() {
		var o models.BundleOption
		if err := rows.Scan(&o.ID, &o.ChoiceID, &o.OptionProductID, &o.DefaultQty); err != nil {
			return nil, err
		}
		list = append(list, o)
	}
	return list, rows.Err()
}
