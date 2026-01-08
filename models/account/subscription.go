package account

import (
	"context"
	"errors"
	"time"

	"github.com/Ptt-Alertor/ptt-alertor/connections"
	"github.com/jackc/pgx/v5"
)

var (
	ErrSubscriptionNotFound = errors.New("subscription not found")
	ErrSubscriptionExists   = errors.New("subscription already exists")
)

// MaxSubscriptionsForUser is the maximum number of subscriptions for regular users
const MaxSubscriptionsForUser = 3

// Subscription represents a user subscription
type Subscription struct {
	ID        int       `json:"id"`
	UserID    int       `json:"user_id"`
	Board     string    `json:"board"`
	SubType   string    `json:"sub_type"`
	Value     string    `json:"value"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// SubscriptionPostgres is the PostgreSQL repository for subscriptions
type SubscriptionPostgres struct{}

// Create creates a new subscription
func (p *SubscriptionPostgres) Create(userID int, board, subType, value string) (*Subscription, error) {
	ctx := context.Background()
	pool := connections.Postgres()

	var sub Subscription
	err := pool.QueryRow(ctx, `
		INSERT INTO subscriptions (user_id, board, sub_type, value)
		VALUES ($1, $2, $3, $4)
		RETURNING id, user_id, board, sub_type, value, enabled, created_at, updated_at
	`, userID, board, subType, value).Scan(
		&sub.ID,
		&sub.UserID,
		&sub.Board,
		&sub.SubType,
		&sub.Value,
		&sub.Enabled,
		&sub.CreatedAt,
		&sub.UpdatedAt,
	)

	if err != nil {
		if err.Error() == `ERROR: duplicate key value violates unique constraint "subscriptions_user_id_board_sub_type_value_key" (SQLSTATE 23505)` {
			return nil, ErrSubscriptionExists
		}
		return nil, err
	}

	return &sub, nil
}

// FindByID finds a subscription by ID
func (p *SubscriptionPostgres) FindByID(id int) (*Subscription, error) {
	ctx := context.Background()
	pool := connections.Postgres()

	var sub Subscription
	err := pool.QueryRow(ctx, `
		SELECT id, user_id, board, sub_type, value, enabled, created_at, updated_at
		FROM subscriptions
		WHERE id = $1
	`, id).Scan(
		&sub.ID,
		&sub.UserID,
		&sub.Board,
		&sub.SubType,
		&sub.Value,
		&sub.Enabled,
		&sub.CreatedAt,
		&sub.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrSubscriptionNotFound
		}
		return nil, err
	}

	return &sub, nil
}

// ListByUserID returns all subscriptions for a user
func (p *SubscriptionPostgres) ListByUserID(userID int) ([]*Subscription, error) {
	ctx := context.Background()
	pool := connections.Postgres()

	rows, err := pool.Query(ctx, `
		SELECT id, user_id, board, sub_type, value, enabled, created_at, updated_at
		FROM subscriptions
		WHERE user_id = $1
		ORDER BY board, sub_type, value
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var subs []*Subscription
	for rows.Next() {
		var sub Subscription
		err := rows.Scan(
			&sub.ID,
			&sub.UserID,
			&sub.Board,
			&sub.SubType,
			&sub.Value,
			&sub.Enabled,
			&sub.CreatedAt,
			&sub.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		subs = append(subs, &sub)
	}

	return subs, nil
}

// Update updates a subscription
func (p *SubscriptionPostgres) Update(id int, enabled bool) error {
	ctx := context.Background()
	pool := connections.Postgres()

	_, err := pool.Exec(ctx, `
		UPDATE subscriptions
		SET enabled = $1
		WHERE id = $2
	`, enabled, id)

	return err
}

// Delete deletes a subscription
func (p *SubscriptionPostgres) Delete(id int) error {
	ctx := context.Background()
	pool := connections.Postgres()

	_, err := pool.Exec(ctx, `
		DELETE FROM subscriptions
		WHERE id = $1
	`, id)

	return err
}

// CountByUserID counts the number of subscriptions for a user
func (p *SubscriptionPostgres) CountByUserID(userID int) (int, error) {
	ctx := context.Background()
	pool := connections.Postgres()

	var count int
	err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM subscriptions WHERE user_id = $1
	`, userID).Scan(&count)

	if err != nil {
		return 0, err
	}

	return count, nil
}

// GetUserAccount gets the account info for a subscription (for Redis sync)
func (p *SubscriptionPostgres) GetUserAccount(subscriptionID int) (*Account, error) {
	ctx := context.Background()
	pool := connections.Postgres()

	var account Account
	err := pool.QueryRow(ctx, `
		SELECT u.id, u.email, u.role, u.enabled
		FROM users u
		JOIN subscriptions s ON s.user_id = u.id
		WHERE s.id = $1
	`, subscriptionID).Scan(
		&account.ID,
		&account.Email,
		&account.Role,
		&account.Enabled,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrAccountNotFound
		}
		return nil, err
	}

	return &account, nil
}
