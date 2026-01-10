package account

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Ptt-Alertor/ptt-alertor/connections"
	"github.com/Ptt-Alertor/ptt-alertor/models/top"
	"github.com/Ptt-Alertor/ptt-alertor/ptt/rss"
	"github.com/jackc/pgx/v5"
)

var (
	ErrSubscriptionNotFound     = errors.New("subscription not found")
	ErrSubscriptionExists       = errors.New("subscription already exists")
	ErrSubscriptionLimitReached = errors.New("subscription limit reached")
	ErrBoardNotFound            = errors.New("board not found")
	ErrUserNotBound             = errors.New("user not bound")
)

// Internal repos for service logic
var (
	roleLimitRepoInternal = &RoleLimitPostgres{}
	accountRepoInternal   = &Postgres{}
	redisSyncInternal     = &RedisSync{}
	statsRepoInternal     = &top.Postgres{}
)

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

// Create creates a new subscription with full logic (validate, DB, Redis sync, stats)
func (p *SubscriptionPostgres) Create(userID int, board, subType, value string) (*Subscription, error) {
	// 1. Get account info
	acc, err := accountRepoInternal.FindByID(userID)
	if err != nil {
		return nil, err
	}

	// 2. Check subscription limit
	if err := p.CheckLimit(userID, acc.Role); err != nil {
		return nil, err
	}

	// 3. Validate board exists
	if !rss.CheckBoardExist(board) {
		return nil, ErrBoardNotFound
	}

	// 4. Create in DB
	sub, err := p.createInDB(userID, board, subType, value)
	if err != nil {
		return nil, err
	}

	// 5. Sync to Redis (async)
	go redisSyncInternal.SyncSubscriptionCreate(sub, acc)

	// 6. Update stats (async)
	go syncStats(board, subType, value, true)

	return sub, nil
}

// createInDB creates a subscription in database only
func (p *SubscriptionPostgres) createInDB(userID int, board, subType, value string) (*Subscription, error) {
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

// CheckLimit checks if user can add more subscriptions
func (p *SubscriptionPostgres) CheckLimit(userID int, role string) error {
	maxSubs, err := roleLimitRepoInternal.GetMaxSubscriptions(role)
	if err != nil {
		return err
	}
	// -1 means unlimited
	if maxSubs < 0 {
		return nil
	}
	count, err := p.CountByUserID(userID)
	if err != nil {
		return err
	}
	if count >= maxSubs {
		return ErrSubscriptionLimitReached
	}
	return nil
}

// syncStats syncs subscription stats (increment or decrement)
func syncStats(board, subType, value string, increment bool) {
	// Article subscriptions don't need stats (articles are ephemeral and personal)
	if subType == "article" {
		return
	}

	var values []string
	if subType == "keyword" {
		values = parseKeywordValues(value)
	} else {
		values = []string{value}
	}
	if len(values) == 0 {
		return
	}
	if increment {
		statsRepoInternal.IncrementBatch(board, subType, values)
	} else {
		statsRepoInternal.DecrementBatch(board, subType, values)
	}
}

// parseKeywordValues parses a keyword value and returns individual keywords
func parseKeywordValues(value string) []string {
	// Skip exclude patterns
	if strings.HasPrefix(value, "!") {
		return nil
	}

	// Handle regexp patterns: regexp:A|B|C -> [A, B, C]
	if pattern, found := strings.CutPrefix(value, "regexp:"); found {
		parts := strings.Split(pattern, "|")
		var result []string
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part != "" {
				result = append(result, part)
			}
		}
		return result
	}

	// Handle AND patterns: A&B -> [A, B]
	if strings.Contains(value, "&") {
		parts := strings.Split(value, "&")
		var result []string
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part != "" {
				result = append(result, part)
			}
		}
		return result
	}

	// Simple keyword
	return []string{value}
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
		ORDER BY updated_at DESC
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

// Update updates a subscription with full logic (validate, DB, Redis sync, stats)
func (p *SubscriptionPostgres) Update(id, userID int, board, subType, value string, enabled bool, mailSubject, mailContent *string) error {
	// 1. Get existing subscription
	sub, err := p.FindByID(id)
	if err != nil {
		return err
	}

	// 2. Check ownership
	if sub.UserID != userID {
		return ErrSubscriptionNotFound
	}

	// 3. Validate board exists
	if !rss.CheckBoardExist(board) {
		return ErrBoardNotFound
	}

	// Store old values for stats
	oldBoard, oldSubType, oldValue := sub.Board, sub.SubType, sub.Value

	// 4. Update in DB
	if err := p.updateInDB(id, board, subType, value, enabled, mailSubject, mailContent); err != nil {
		return err
	}

	// 5. Update sub for Redis sync
	sub.Board = board
	sub.SubType = subType
	sub.Value = value
	sub.Enabled = enabled

	// 6. Sync to Redis (async)
	acc, _ := accountRepoInternal.FindByID(userID)
	if acc != nil {
		go redisSyncInternal.SyncSubscriptionCreate(sub, acc)
	}

	// 7. Update stats (async) - decrement old, increment new
	go func() {
		syncStats(oldBoard, oldSubType, oldValue, false)
		syncStats(board, subType, value, true)
	}()

	return nil
}

// updateInDB updates a subscription in database only
func (p *SubscriptionPostgres) updateInDB(id int, board, subType, value string, enabled bool, mailSubject, mailContent *string) error {
	ctx := context.Background()
	pool := connections.Postgres()

	_, err := pool.Exec(ctx, `
		UPDATE subscriptions
		SET board = $1, sub_type = $2, value = $3, enabled = $4, mail_subject = $5, mail_content = $6, updated_at = NOW()
		WHERE id = $7
	`, board, subType, value, enabled, mailSubject, mailContent, id)

	return err
}

// Delete deletes a subscription with full logic (DB, Redis sync, stats)
func (p *SubscriptionPostgres) Delete(id, userID int) error {
	// 1. Get subscription info for sync
	sub, err := p.FindByID(id)
	if err != nil {
		return err
	}

	// 2. Check ownership
	if sub.UserID != userID {
		return ErrSubscriptionNotFound
	}

	// 3. Get account for Redis sync
	acc, _ := accountRepoInternal.FindByID(userID)

	// 4. Delete from DB
	if err := p.deleteInDB(id); err != nil {
		return err
	}

	// 5. Sync to Redis (async)
	if acc != nil {
		go redisSyncInternal.SyncSubscriptionDelete(sub, acc)
	}

	// 6. Update stats (async)
	go syncStats(sub.Board, sub.SubType, sub.Value, false)

	return nil
}

// deleteInDB deletes a subscription from database only
func (p *SubscriptionPostgres) deleteInDB(id int) error {
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

// CountByUserIDAndType counts the number of subscriptions for a user by type
func (p *SubscriptionPostgres) CountByUserIDAndType(userID int, subType string) (int, error) {
	ctx := context.Background()
	pool := connections.Postgres()

	var count int
	err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM subscriptions WHERE user_id = $1 AND sub_type = $2
	`, userID, subType).Scan(&count)

	if err != nil {
		return 0, err
	}

	return count, nil
}

// ListByUserIDAndType returns all subscriptions for a user by type
func (p *SubscriptionPostgres) ListByUserIDAndType(userID int, subType string) ([]*Subscription, error) {
	ctx := context.Background()
	pool := connections.Postgres()

	rows, err := pool.Query(ctx, `
		SELECT id, user_id, board, sub_type, value, enabled, mail_subject, mail_content, created_at, updated_at
		FROM subscriptions
		WHERE user_id = $1 AND sub_type = $2
		ORDER BY updated_at DESC
	`, userID, subType)
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

// GetUserIDByTelegramChatID gets userID from Telegram chat ID via binding
func GetUserIDByTelegramChatID(chatID string) (int, error) {
	ctx := context.Background()
	pool := connections.Postgres()

	var userID int
	err := pool.QueryRow(ctx, `
		SELECT user_id FROM notification_bindings
		WHERE service = 'telegram' AND service_id = $1
	`, chatID).Scan(&userID)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, ErrUserNotBound
		}
		return 0, err
	}
	return userID, nil
}

// FindByBoardAndValue finds a subscription by userID, board, subType, and value
func (p *SubscriptionPostgres) FindByBoardAndValue(userID int, board, subType, value string) (*Subscription, error) {
	ctx := context.Background()
	pool := connections.Postgres()

	var sub Subscription
	var mailSubject, mailContent *string
	err := pool.QueryRow(ctx, `
		SELECT id, user_id, board, sub_type, value, enabled, mail_subject, mail_content, created_at, updated_at
		FROM subscriptions
		WHERE user_id = $1 AND LOWER(board) = LOWER($2) AND sub_type = $3 AND value = $4
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
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrSubscriptionNotFound
		}
		return nil, err
	}

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

// DeleteByValue deletes a subscription by board, subType, and value for Telegram Bot
func (p *SubscriptionPostgres) DeleteByValue(userID int, board, subType, value string) error {
	sub, err := p.FindByBoardAndValue(userID, board, subType, value)
	if err != nil {
		return err
	}
	return p.Delete(sub.ID, userID)
}

// ListFormatted returns formatted subscription list for Telegram Bot
func (p *SubscriptionPostgres) ListFormatted(userID int) (string, error) {
	subs, err := p.ListByUserID(userID)
	if err != nil {
		return "", err
	}

	if len(subs) == 0 {
		return "尚未建立訂閱清單。", nil
	}

	// Group by board and sub_type
	keywords := make(map[string][]string)   // board -> keywords
	authors := make(map[string][]string)    // board -> authors
	pushsums := make(map[string]string)     // board -> pushsum value

	for _, sub := range subs {
		if !sub.Enabled {
			continue
		}
		switch sub.SubType {
		case "keyword":
			keywords[sub.Board] = append(keywords[sub.Board], sub.Value)
		case "author":
			authors[sub.Board] = append(authors[sub.Board], sub.Value)
		case "pushsum":
			pushsums[sub.Board] = sub.Value
		}
	}

	var result strings.Builder

	// Format keywords
	result.WriteString("關鍵字\n")
	if len(keywords) > 0 {
		boards := make([]string, 0, len(keywords))
		for board := range keywords {
			boards = append(boards, board)
		}
		sort.Strings(boards)
		for _, board := range boards {
			sort.Strings(keywords[board])
			result.WriteString(fmt.Sprintf("%s: %s\n", board, strings.Join(keywords[board], ", ")))
		}
	}

	// Format authors
	result.WriteString("----\n作者\n")
	if len(authors) > 0 {
		boards := make([]string, 0, len(authors))
		for board := range authors {
			boards = append(boards, board)
		}
		sort.Strings(boards)
		for _, board := range boards {
			sort.Strings(authors[board])
			result.WriteString(fmt.Sprintf("%s: %s\n", board, strings.Join(authors[board], ", ")))
		}
	}

	// Format pushsums
	result.WriteString("----\n推文數\n")
	if len(pushsums) > 0 {
		boards := make([]string, 0, len(pushsums))
		for board := range pushsums {
			boards = append(boards, board)
		}
		sort.Strings(boards)
		for _, board := range boards {
			result.WriteString(fmt.Sprintf("%s: %s\n", board, pushsums[board]))
		}
	}

	return strings.TrimSpace(result.String()), nil
}
