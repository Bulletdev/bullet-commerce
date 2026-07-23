package addresses

import (
	"bullet-commerce/internal/models"
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DBPool interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Begin(ctx context.Context) (pgx.Tx, error)
}

var (
	ErrAddressNotFound = errors.New("address not found")
	// ErrForbidden is used when a user tries to access/modify an address not belonging to them (handled potentially in handler layer)
)

// AddressRepository defines the interface for address data operations.
type AddressRepository interface {
	Create(ctx context.Context, address *models.Address) (*models.Address, error)
	FindByUserID(ctx context.Context, userID uuid.UUID) ([]models.Address, error)
	FindByUserAndID(ctx context.Context, userID, addressID uuid.UUID) (*models.Address, error)
	Update(ctx context.Context, userID, addressID uuid.UUID, address *models.Address) (*models.Address, error)
	Delete(ctx context.Context, userID, addressID uuid.UUID) error
	SetDefault(ctx context.Context, userID, addressID uuid.UUID) error
	SetDefaultBilling(ctx context.Context, userID, addressID uuid.UUID) error
	SetDefaultShipping(ctx context.Context, userID, addressID uuid.UUID) error
}

// postgresAddressRepository implements AddressRepository using PostgreSQL.
type postgresAddressRepository struct {
	db DBPool
}

func NewPostgresAddressRepository(db *pgxpool.Pool) AddressRepository {
	return &postgresAddressRepository{db: db}
}

// Create inserts a new address for a user into the database.
func (r *postgresAddressRepository) Create(ctx context.Context, address *models.Address) (*models.Address, error) {
	// If this new address is set as default, unset other defaults for this user first
	if address.IsDefault {
		err := r.unsetDefaultAddresses(ctx, address.UserID)
		if err != nil {
			return nil, err
		}
	}

	query := `
		INSERT INTO addresses (user_id, street, city, state, postal_code, country, is_default)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at, updated_at
	`
	err := r.db.QueryRow(ctx, query,
		address.UserID,
		address.Street,
		address.City,
		address.State,
		address.PostalCode,
		address.Country,
		address.IsDefault,
	).Scan(&address.ID, &address.CreatedAt, &address.UpdatedAt)

	if err != nil {
		// TODO: Handle specific errors like FK violation if user_id doesn't exist
		return nil, err
	}
	return address, nil
}

// FindByUserID retrieves all addresses for a specific user.
func (r *postgresAddressRepository) FindByUserID(ctx context.Context, userID uuid.UUID) ([]models.Address, error) {
	query := `
		SELECT id, user_id, street, city, state, postal_code, country, is_default, is_default_billing, is_default_shipping, created_at, updated_at
		FROM addresses
		WHERE user_id = $1
		ORDER BY is_default DESC, created_at DESC -- Show default first, then newest
	`
	rows, err := r.db.Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	addresses, err := pgx.CollectRows(rows, pgx.RowToStructByName[models.Address])
	if err != nil {
		return nil, err
	}
	// It's okay to return an empty slice if the user has no addresses
	return addresses, nil
}

// FindByUserAndID retrieves a specific address for a specific user.
func (r *postgresAddressRepository) FindByUserAndID(ctx context.Context, userID, addressID uuid.UUID) (*models.Address, error) {
	query := `
		SELECT id, user_id, street, city, state, postal_code, country, is_default, is_default_billing, is_default_shipping, created_at, updated_at
		FROM addresses
		WHERE id = $1 AND user_id = $2
	`
	address := &models.Address{}
	err := r.db.QueryRow(ctx, query, addressID, userID).Scan(
		&address.ID,
		&address.UserID,
		&address.Street,
		&address.City,
		&address.State,
		&address.PostalCode,
		&address.Country,
		&address.IsDefault,
		&address.IsDefaultBilling,
		&address.IsDefaultShipping,
		&address.CreatedAt,
		&address.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrAddressNotFound // Address not found or doesn't belong to user
		}
		return nil, err
	}
	return address, nil
}

// Update modifies an existing address for a specific user.
func (r *postgresAddressRepository) Update(ctx context.Context, userID, addressID uuid.UUID, address *models.Address) (*models.Address, error) {
	// If this address is being set as default, unset others first
	if address.IsDefault {
		err := r.unsetDefaultAddresses(ctx, userID)
		if err != nil {
			return nil, err
		}
	}

	query := `
		UPDATE addresses
		SET street = $1, city = $2, state = $3, postal_code = $4, country = $5, is_default = $6, updated_at = NOW()
		WHERE id = $7 AND user_id = $8
		RETURNING updated_at
	`
	err := r.db.QueryRow(ctx, query,
		address.Street,
		address.City,
		address.State,
		address.PostalCode,
		address.Country,
		address.IsDefault,
		addressID,
		userID,
	).Scan(&address.UpdatedAt)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrAddressNotFound // Address not found or doesn't belong to user
		}
		return nil, err
	}

	// Fill in potentially unchanged data
	address.ID = addressID
	address.UserID = userID
	return address, nil
}

// Delete removes a specific address for a specific user.
func (r *postgresAddressRepository) Delete(ctx context.Context, userID, addressID uuid.UUID) error {
	query := `DELETE FROM addresses WHERE id = $1 AND user_id = $2`
	result, err := r.db.Exec(ctx, query, addressID, userID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrAddressNotFound // Address not found or doesn't belong to user
	}
	return nil
}

// SetDefault marks a specific address as the default for the user,
// unsetting any other default addresses for that user.
func (r *postgresAddressRepository) SetDefault(ctx context.Context, userID, addressID uuid.UUID) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) // Rollback if anything fails

	// Unset other defaults for this user
	unsetQuery := `UPDATE addresses SET is_default = false WHERE user_id = $1 AND is_default = true`
	_, err = tx.Exec(ctx, unsetQuery, userID)
	if err != nil {
		return err
	}

	// Set the specified address as default
	setQuery := `UPDATE addresses SET is_default = true, updated_at = NOW() WHERE id = $1 AND user_id = $2`
	result, err := tx.Exec(ctx, setQuery, addressID, userID)
	if err != nil {
		return err
	}

	// Check if the target address existed and belonged to the user
	if result.RowsAffected() == 0 {
		return ErrAddressNotFound
	}

	return tx.Commit(ctx) // Commit the transaction
}

// SetDefaultBilling marks a specific address as the user's default billing address.
// Unset and set run in one transaction so the per-user single-billing-default invariant
// is never briefly violated (which the partial unique index would otherwise reject).
func (r *postgresAddressRepository) SetDefaultBilling(ctx context.Context, userID, addressID uuid.UUID) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	unsetQuery := `UPDATE addresses SET is_default_billing = false WHERE user_id = $1 AND is_default_billing = true`
	if _, err = tx.Exec(ctx, unsetQuery, userID); err != nil {
		return err
	}

	setQuery := `UPDATE addresses SET is_default_billing = true, updated_at = NOW() WHERE id = $1 AND user_id = $2`
	result, err := tx.Exec(ctx, setQuery, addressID, userID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrAddressNotFound
	}

	return tx.Commit(ctx)
}

// SetDefaultShipping marks a specific address as the user's default shipping address.
// Independent from billing: changing the shipping default leaves the billing default untouched.
func (r *postgresAddressRepository) SetDefaultShipping(ctx context.Context, userID, addressID uuid.UUID) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	unsetQuery := `UPDATE addresses SET is_default_shipping = false WHERE user_id = $1 AND is_default_shipping = true`
	if _, err = tx.Exec(ctx, unsetQuery, userID); err != nil {
		return err
	}

	setQuery := `UPDATE addresses SET is_default_shipping = true, updated_at = NOW() WHERE id = $1 AND user_id = $2`
	result, err := tx.Exec(ctx, setQuery, addressID, userID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrAddressNotFound
	}

	return tx.Commit(ctx)
}

// unsetDefaultAddresses is a helper to set is_default=false for all addresses of a user.
// This should typically be called within a transaction.
func (r *postgresAddressRepository) unsetDefaultAddresses(ctx context.Context, userID uuid.UUID) error {
	query := `UPDATE addresses SET is_default = false WHERE user_id = $1 AND is_default = true`
	_, err := r.db.Exec(ctx, query, userID) // Use db directly or pass tx if called within transaction
	return err
}
