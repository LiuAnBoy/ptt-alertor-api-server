package binding

import (
	"context"
	"errors"
	"time"

	"github.com/Ptt-Alertor/ptt-alertor/connections"
	"github.com/jackc/pgx/v5"
)

// Postgres implements Repository interface
type Postgres struct{}

// Create creates a new binding
func (p *Postgres) Create(userID int, service, serviceID string) (*NotificationBinding, error) {
	ctx := context.Background()
	pool := connections.Postgres()

	var binding NotificationBinding
	err := pool.QueryRow(ctx, `
		INSERT INTO notification_bindings (user_id, service, service_id, enabled)
		VALUES ($1, $2, $3, true)
		RETURNING id, user_id, service, service_id, enabled, created_at, updated_at
	`, userID, service, serviceID).Scan(
		&binding.ID,
		&binding.UserID,
		&binding.Service,
		&binding.ServiceID,
		&binding.Enabled,
		&binding.CreatedAt,
		&binding.UpdatedAt,
	)

	if err != nil {
		return nil, err
	}

	return &binding, nil
}

// FindByUserAndService finds binding by user ID and service type
func (p *Postgres) FindByUserAndService(userID int, service string) (*NotificationBinding, error) {
	ctx := context.Background()
	pool := connections.Postgres()

	var binding NotificationBinding
	err := pool.QueryRow(ctx, `
		SELECT id, user_id, service, service_id, bind_code, bind_code_expires_at, enabled, created_at, updated_at
		FROM notification_bindings
		WHERE user_id = $1 AND service = $2
	`, userID, service).Scan(
		&binding.ID,
		&binding.UserID,
		&binding.Service,
		&binding.ServiceID,
		&binding.BindCode,
		&binding.BindCodeExpiresAt,
		&binding.Enabled,
		&binding.CreatedAt,
		&binding.UpdatedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrBindingNotFound
	}
	if err != nil {
		return nil, err
	}

	return &binding, nil
}

// FindByServiceID finds binding by service type and service ID
func (p *Postgres) FindByServiceID(service, serviceID string) (*NotificationBinding, error) {
	ctx := context.Background()
	pool := connections.Postgres()

	var binding NotificationBinding
	err := pool.QueryRow(ctx, `
		SELECT id, user_id, service, service_id, bind_code, bind_code_expires_at, enabled, created_at, updated_at
		FROM notification_bindings
		WHERE service = $1 AND service_id = $2
	`, service, serviceID).Scan(
		&binding.ID,
		&binding.UserID,
		&binding.Service,
		&binding.ServiceID,
		&binding.BindCode,
		&binding.BindCodeExpiresAt,
		&binding.Enabled,
		&binding.CreatedAt,
		&binding.UpdatedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrBindingNotFound
	}
	if err != nil {
		return nil, err
	}

	return &binding, nil
}

// FindAllByUser finds all bindings for a user
func (p *Postgres) FindAllByUser(userID int) ([]*NotificationBinding, error) {
	ctx := context.Background()
	pool := connections.Postgres()

	rows, err := pool.Query(ctx, `
		SELECT id, user_id, service, service_id, enabled, created_at, updated_at
		FROM notification_bindings
		WHERE user_id = $1
		ORDER BY service
	`, userID)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var bindings []*NotificationBinding
	for rows.Next() {
		var binding NotificationBinding
		err := rows.Scan(
			&binding.ID,
			&binding.UserID,
			&binding.Service,
			&binding.ServiceID,
			&binding.Enabled,
			&binding.CreatedAt,
			&binding.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		bindings = append(bindings, &binding)
	}

	return bindings, nil
}

// UpdateBindCode updates the bind code for pending binding
func (p *Postgres) UpdateBindCode(userID int, service, code string, expiresAt time.Time) error {
	ctx := context.Background()
	pool := connections.Postgres()

	// First try to update existing record
	result, err := pool.Exec(ctx, `
		UPDATE notification_bindings
		SET bind_code = $1, bind_code_expires_at = $2, updated_at = CURRENT_TIMESTAMP
		WHERE user_id = $3 AND service = $4
	`, code, expiresAt, userID, service)

	if err != nil {
		return err
	}

	// If no rows updated, create a new record with empty service_id
	if result.RowsAffected() == 0 {
		_, err = pool.Exec(ctx, `
			INSERT INTO notification_bindings (user_id, service, service_id, bind_code, bind_code_expires_at, enabled)
			VALUES ($1, $2, '', $3, $4, false)
		`, userID, service, code, expiresAt)
		if err != nil {
			return err
		}
	}

	return nil
}

// ConfirmBinding confirms binding with service ID
func (p *Postgres) ConfirmBinding(userID int, service, serviceID string) error {
	ctx := context.Background()
	pool := connections.Postgres()

	_, err := pool.Exec(ctx, `
		UPDATE notification_bindings
		SET service_id = $1, bind_code = NULL, bind_code_expires_at = NULL, enabled = true, updated_at = CURRENT_TIMESTAMP
		WHERE user_id = $2 AND service = $3
	`, serviceID, userID, service)

	return err
}

// Delete deletes a binding
func (p *Postgres) Delete(userID int, service string) error {
	ctx := context.Background()
	pool := connections.Postgres()

	result, err := pool.Exec(ctx, `
		DELETE FROM notification_bindings
		WHERE user_id = $1 AND service = $2
	`, userID, service)

	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return ErrBindingNotFound
	}

	return nil
}

// SetEnabled enables or disables a binding
func (p *Postgres) SetEnabled(userID int, service string, enabled bool) error {
	ctx := context.Background()
	pool := connections.Postgres()

	result, err := pool.Exec(ctx, `
		UPDATE notification_bindings
		SET enabled = $1, updated_at = CURRENT_TIMESTAMP
		WHERE user_id = $2 AND service = $3
	`, enabled, userID, service)

	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return ErrBindingNotFound
	}

	return nil
}

// FindByBindCode finds binding by bind code
func (p *Postgres) FindByBindCode(service, code string) (*NotificationBinding, error) {
	ctx := context.Background()
	pool := connections.Postgres()

	var binding NotificationBinding
	err := pool.QueryRow(ctx, `
		SELECT id, user_id, service, service_id, bind_code, bind_code_expires_at, enabled, created_at, updated_at
		FROM notification_bindings
		WHERE service = $1 AND bind_code = $2 AND bind_code_expires_at > CURRENT_TIMESTAMP
	`, service, code).Scan(
		&binding.ID,
		&binding.UserID,
		&binding.Service,
		&binding.ServiceID,
		&binding.BindCode,
		&binding.BindCodeExpiresAt,
		&binding.Enabled,
		&binding.CreatedAt,
		&binding.UpdatedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrBindingNotFound
	}
	if err != nil {
		return nil, err
	}

	return &binding, nil
}
