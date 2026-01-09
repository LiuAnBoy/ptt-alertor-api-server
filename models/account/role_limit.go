package account

import (
	"context"
	"errors"
	"time"

	"github.com/Ptt-Alertor/ptt-alertor/connections"
	"github.com/jackc/pgx/v5"
)

var (
	ErrRoleLimitNotFound = errors.New("role limit not found")
	ErrRoleInUse         = errors.New("role is in use by users")
)

// RoleLimit represents a role's subscription limit configuration
type RoleLimit struct {
	Role             string    `json:"role"`
	MaxSubscriptions int       `json:"max_subscriptions"`
	Description      string    `json:"description"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// RoleLimitPostgres is the PostgreSQL repository for role limits
type RoleLimitPostgres struct{}

// GetMaxSubscriptions returns the max subscriptions for a role
// Returns -1 for unlimited, or the limit value
func (p *RoleLimitPostgres) GetMaxSubscriptions(role string) (int, error) {
	ctx := context.Background()
	pool := connections.Postgres()

	var maxSubs int
	err := pool.QueryRow(ctx, `
		SELECT max_subscriptions FROM role_limits WHERE role = $1
	`, role).Scan(&maxSubs)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Default to 3 if role not found
			return 3, nil
		}
		return 3, err
	}

	return maxSubs, nil
}

// FindByRole finds a role limit by role name
func (p *RoleLimitPostgres) FindByRole(role string) (*RoleLimit, error) {
	ctx := context.Background()
	pool := connections.Postgres()

	var rl RoleLimit
	err := pool.QueryRow(ctx, `
		SELECT role, max_subscriptions, description, created_at, updated_at
		FROM role_limits
		WHERE role = $1
	`, role).Scan(
		&rl.Role,
		&rl.MaxSubscriptions,
		&rl.Description,
		&rl.CreatedAt,
		&rl.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRoleLimitNotFound
		}
		return nil, err
	}

	return &rl, nil
}

// List returns all role limits
func (p *RoleLimitPostgres) List() ([]*RoleLimit, error) {
	ctx := context.Background()
	pool := connections.Postgres()

	rows, err := pool.Query(ctx, `
		SELECT role, max_subscriptions, description, created_at, updated_at
		FROM role_limits
		ORDER BY
			CASE role
				WHEN 'admin' THEN 1
				WHEN 'vip' THEN 2
				ELSE 3
			END
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var limits []*RoleLimit
	for rows.Next() {
		var rl RoleLimit
		err := rows.Scan(
			&rl.Role,
			&rl.MaxSubscriptions,
			&rl.Description,
			&rl.CreatedAt,
			&rl.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		limits = append(limits, &rl)
	}

	return limits, nil
}

// Create creates a new role limit
func (p *RoleLimitPostgres) Create(role string, maxSubs int, description string) (*RoleLimit, error) {
	ctx := context.Background()
	pool := connections.Postgres()

	var rl RoleLimit
	err := pool.QueryRow(ctx, `
		INSERT INTO role_limits (role, max_subscriptions, description)
		VALUES ($1, $2, $3)
		RETURNING role, max_subscriptions, description, created_at, updated_at
	`, role, maxSubs, description).Scan(
		&rl.Role,
		&rl.MaxSubscriptions,
		&rl.Description,
		&rl.CreatedAt,
		&rl.UpdatedAt,
	)

	if err != nil {
		return nil, err
	}

	return &rl, nil
}

// Update updates a role limit
func (p *RoleLimitPostgres) Update(role string, maxSubs int, description string) (*RoleLimit, error) {
	ctx := context.Background()
	pool := connections.Postgres()

	var rl RoleLimit
	err := pool.QueryRow(ctx, `
		UPDATE role_limits
		SET max_subscriptions = $2, description = $3
		WHERE role = $1
		RETURNING role, max_subscriptions, description, created_at, updated_at
	`, role, maxSubs, description).Scan(
		&rl.Role,
		&rl.MaxSubscriptions,
		&rl.Description,
		&rl.CreatedAt,
		&rl.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRoleLimitNotFound
		}
		return nil, err
	}

	return &rl, nil
}

// Delete deletes a role limit (only if no users are using it)
func (p *RoleLimitPostgres) Delete(role string) error {
	ctx := context.Background()
	pool := connections.Postgres()

	// Check if any users are using this role
	var count int
	err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM users WHERE role = $1
	`, role).Scan(&count)
	if err != nil {
		return err
	}
	if count > 0 {
		return ErrRoleInUse
	}

	result, err := pool.Exec(ctx, `
		DELETE FROM role_limits WHERE role = $1
	`, role)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return ErrRoleLimitNotFound
	}

	return nil
}

// CountUsersByRole returns the number of users with a specific role
func (p *RoleLimitPostgres) CountUsersByRole(role string) (int, error) {
	ctx := context.Background()
	pool := connections.Postgres()

	var count int
	err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM users WHERE role = $1
	`, role).Scan(&count)

	return count, err
}

// GetMaxSubscriptionsByServiceID returns max subscriptions for a user by their notification service ID
// This is used for chatbot commands (Telegram, LINE) where we need to look up the user's role
// Returns -1 for unlimited, or the limit value. Returns default (3) if user not found.
func (p *RoleLimitPostgres) GetMaxSubscriptionsByServiceID(service, serviceID string) (int, error) {
	ctx := context.Background()
	pool := connections.Postgres()

	var maxSubs int
	err := pool.QueryRow(ctx, `
		SELECT rl.max_subscriptions
		FROM notification_bindings nb
		JOIN users u ON nb.user_id = u.id
		JOIN role_limits rl ON u.role = rl.role
		WHERE nb.service = $1 AND nb.service_id = $2
	`, service, serviceID).Scan(&maxSubs)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// User not bound to any account, return default
			return 3, nil
		}
		return 3, err
	}

	return maxSubs, nil
}
