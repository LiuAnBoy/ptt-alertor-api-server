package account

import (
	"context"
	"errors"

	"github.com/Ptt-Alertor/ptt-alertor/connections"
	"github.com/jackc/pgx/v5"
)

// Postgres is the PostgreSQL repository for users
type Postgres struct{}

// Create creates a new account
func (p *Postgres) Create(email, passwordHash, role string) (*Account, error) {
	ctx := context.Background()
	pool := connections.Postgres()

	var account Account
	err := pool.QueryRow(ctx, `
		INSERT INTO users (email, password, role)
		VALUES ($1, $2, $3)
		RETURNING id, email, role, enabled, created_at, updated_at
	`, email, passwordHash, role).Scan(
		&account.ID,
		&account.Email,
		&account.Role,
		&account.Enabled,
		&account.CreatedAt,
		&account.UpdatedAt,
	)

	if err != nil {
		if err.Error() == `ERROR: duplicate key value violates unique constraint "users_email_key" (SQLSTATE 23505)` {
			return nil, ErrEmailExists
		}
		return nil, err
	}

	return &account, nil
}

// FindByEmail finds an account by email
func (p *Postgres) FindByEmail(email string) (*Account, error) {
	ctx := context.Background()
	pool := connections.Postgres()

	var account Account
	err := pool.QueryRow(ctx, `
		SELECT id, email, password, role, enabled, created_at, updated_at
		FROM users
		WHERE email = $1
	`, email).Scan(
		&account.ID,
		&account.Email,
		&account.Password,
		&account.Role,
		&account.Enabled,
		&account.CreatedAt,
		&account.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrAccountNotFound
		}
		return nil, err
	}

	return &account, nil
}

// FindByID finds an account by ID
func (p *Postgres) FindByID(id int) (*Account, error) {
	ctx := context.Background()
	pool := connections.Postgres()

	var account Account
	err := pool.QueryRow(ctx, `
		SELECT id, email, password, role, enabled, created_at, updated_at
		FROM users
		WHERE id = $1
	`, id).Scan(
		&account.ID,
		&account.Email,
		&account.Password,
		&account.Role,
		&account.Enabled,
		&account.CreatedAt,
		&account.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrAccountNotFound
		}
		return nil, err
	}

	return &account, nil
}

// ListResult represents paginated list result
type ListResult struct {
	Users []*Account `json:"users"`
	Total int        `json:"total"`
	Page  int        `json:"page"`
	Limit int        `json:"limit"`
}

// List returns users with pagination and search (for admin)
func (p *Postgres) List(query string, page, limit int) (*ListResult, error) {
	ctx := context.Background()
	pool := connections.Postgres()

	// Default values
	if page <= 0 {
		page = 1
	}
	if limit <= 0 {
		limit = 20
	}

	offset := (page - 1) * limit

	var total int
	var rows pgx.Rows
	var err error

	if query != "" {
		// Search by email (case-insensitive)
		searchPattern := "%" + query + "%"

		// Get total count
		err = pool.QueryRow(ctx, `
			SELECT COUNT(*) FROM users WHERE email ILIKE $1
		`, searchPattern).Scan(&total)
		if err != nil {
			return nil, err
		}

		// Get paginated results
		rows, err = pool.Query(ctx, `
			SELECT id, email, role, enabled, created_at, updated_at
			FROM users
			WHERE email ILIKE $1
			ORDER BY created_at DESC
			LIMIT $2 OFFSET $3
		`, searchPattern, limit, offset)
	} else {
		// Get total count
		err = pool.QueryRow(ctx, `SELECT COUNT(*) FROM users`).Scan(&total)
		if err != nil {
			return nil, err
		}

		// Get paginated results
		rows, err = pool.Query(ctx, `
			SELECT id, email, role, enabled, created_at, updated_at
			FROM users
			ORDER BY created_at DESC
			LIMIT $1 OFFSET $2
		`, limit, offset)
	}

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []*Account
	for rows.Next() {
		var account Account
		err := rows.Scan(
			&account.ID,
			&account.Email,
			&account.Role,
			&account.Enabled,
			&account.CreatedAt,
			&account.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		users = append(users, &account)
	}

	if users == nil {
		users = []*Account{}
	}

	return &ListResult{
		Users: users,
		Total: total,
		Page:  page,
		Limit: limit,
	}, nil
}

// Update updates an account
func (p *Postgres) Update(id int, role string, enabled bool) error {
	ctx := context.Background()
	pool := connections.Postgres()

	_, err := pool.Exec(ctx, `
		UPDATE users
		SET role = $1, enabled = $2
		WHERE id = $3
	`, role, enabled, id)

	return err
}

// UpdatePassword updates user password
func (p *Postgres) UpdatePassword(id int, passwordHash string) error {
	ctx := context.Background()
	pool := connections.Postgres()

	_, err := pool.Exec(ctx, `
		UPDATE users
		SET password_hash = $1
		WHERE id = $2
	`, passwordHash, id)

	return err
}

// Delete deletes an account
func (p *Postgres) Delete(id int) error {
	ctx := context.Background()
	pool := connections.Postgres()

	_, err := pool.Exec(ctx, `
		DELETE FROM users
		WHERE id = $1
	`, id)

	return err
}
