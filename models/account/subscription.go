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

// MailTemplate represents the mail template for a subscription
type MailTemplate struct {
	Subject string `json:"subject,omitempty"`
	Content string `json:"content,omitempty"`
}

// Subscription represents a user subscription
type Subscription struct {
	ID        int           `json:"id"`
	UserID    int           `json:"user_id"`
	Board     string        `json:"board"`
	SubType   string        `json:"sub_type"`
	Value     string        `json:"value"`
	Enabled   bool          `json:"enabled"`
	Mail      *MailTemplate `json:"mail,omitempty"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`
}

// SubscriptionPostgres is the PostgreSQL repository for subscriptions
type SubscriptionPostgres struct{}

// Create creates a new subscription
func (p *SubscriptionPostgres) Create(userID int, board, subType, value string) (*Subscription, error) {
	ctx := context.Background()
	pool := connections.Postgres()

	var sub Subscription
	var mailSubject, mailContent *string
	err := pool.QueryRow(ctx, `
		INSERT INTO subscriptions (user_id, board, sub_type, value)
		VALUES ($1, $2, $3, $4)
		RETURNING id, user_id, board, sub_type, value, enabled, mail_subject, mail_content, created_at, updated_at
	`, userID, board, subType, value).Scan(
		&sub.ID,
		&sub.UserID,
		&sub.Board,
		&sub.SubType,
		&sub.Value,
		&sub.Enabled,
		&mailSubject,
		&mailContent,
		&sub.CreatedAt,
		&sub.UpdatedAt,
	)

	if err != nil {
		if err.Error() == `ERROR: duplicate key value violates unique constraint "subscriptions_user_id_board_sub_type_value_key" (SQLSTATE 23505)` {
			return nil, ErrSubscriptionExists
		}
		return nil, err
	}

	// Set mail template if exists
	if mailSubject != nil || mailContent != nil {
		sub.Mail = &MailTemplate{}
		if mailSubject != nil {
			sub.Mail.Subject = *mailSubject
		}
		if mailContent != nil {
			sub.Mail.Content = *mailContent
		}
	}

	return &sub, nil
}

// FindByID finds a subscription by ID
func (p *SubscriptionPostgres) FindByID(id int) (*Subscription, error) {
	ctx := context.Background()
	pool := connections.Postgres()

	var sub Subscription
	var mailSubject, mailContent *string
	err := pool.QueryRow(ctx, `
		SELECT id, user_id, board, sub_type, value, enabled, mail_subject, mail_content, created_at, updated_at
		FROM subscriptions
		WHERE id = $1
	`, id).Scan(
		&sub.ID,
		&sub.UserID,
		&sub.Board,
		&sub.SubType,
		&sub.Value,
		&sub.Enabled,
		&mailSubject,
		&mailContent,
		&sub.CreatedAt,
		&sub.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrSubscriptionNotFound
		}
		return nil, err
	}

	// Set mail template if exists
	if mailSubject != nil || mailContent != nil {
		sub.Mail = &MailTemplate{}
		if mailSubject != nil {
			sub.Mail.Subject = *mailSubject
		}
		if mailContent != nil {
			sub.Mail.Content = *mailContent
		}
	}

	return &sub, nil
}

// ListByUserID returns all subscriptions for a user
func (p *SubscriptionPostgres) ListByUserID(userID int) ([]*Subscription, error) {
	ctx := context.Background()
	pool := connections.Postgres()

	rows, err := pool.Query(ctx, `
		SELECT id, user_id, board, sub_type, value, enabled, mail_subject, mail_content, created_at, updated_at
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
		var mailSubject, mailContent *string
		err := rows.Scan(
			&sub.ID,
			&sub.UserID,
			&sub.Board,
			&sub.SubType,
			&sub.Value,
			&sub.Enabled,
			&mailSubject,
			&mailContent,
			&sub.CreatedAt,
			&sub.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		// Set mail template if exists
		if mailSubject != nil || mailContent != nil {
			sub.Mail = &MailTemplate{}
			if mailSubject != nil {
				sub.Mail.Subject = *mailSubject
			}
			if mailContent != nil {
				sub.Mail.Content = *mailContent
			}
		}
		subs = append(subs, &sub)
	}

	return subs, nil
}

// Update updates a subscription with all fields
func (p *SubscriptionPostgres) Update(id int, board, subType, value string, enabled bool) error {
	ctx := context.Background()
	pool := connections.Postgres()

	_, err := pool.Exec(ctx, `
		UPDATE subscriptions
		SET board = $1, sub_type = $2, value = $3, enabled = $4, updated_at = NOW()
		WHERE id = $5
	`, board, subType, value, enabled, id)

	return err
}

// UpdateWithMail updates a subscription with all fields including mail template
func (p *SubscriptionPostgres) UpdateWithMail(id int, board, subType, value string, enabled bool, mailSubject, mailContent *string) error {
	ctx := context.Background()
	pool := connections.Postgres()

	_, err := pool.Exec(ctx, `
		UPDATE subscriptions
		SET board = $1, sub_type = $2, value = $3, enabled = $4, mail_subject = $5, mail_content = $6, updated_at = NOW()
		WHERE id = $7
	`, board, subType, value, enabled, mailSubject, mailContent, id)

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
