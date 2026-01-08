package account

import (
	"context"
	"strings"
	"time"

	"github.com/Ptt-Alertor/ptt-alertor/connections"
)

// SubscriptionStat represents a subscription statistic entry
type SubscriptionStat struct {
	ID        int       `json:"id"`
	Board     string    `json:"board"`
	SubType   string    `json:"sub_type"`
	Value     string    `json:"value"`
	Count     int       `json:"count"`
	UpdatedAt time.Time `json:"updated_at"`
}

// SubscriptionStatsPostgres is the PostgreSQL repository for subscription stats
type SubscriptionStatsPostgres struct{}

// Increment increments the count for a board/sub_type/value combination
func (p *SubscriptionStatsPostgres) Increment(board, subType, value string) error {
	ctx := context.Background()
	pool := connections.Postgres()

	_, err := pool.Exec(ctx, `
		INSERT INTO subscription_stats (board, sub_type, value, count)
		VALUES ($1, $2, $3, 1)
		ON CONFLICT (board, sub_type, value)
		DO UPDATE SET count = subscription_stats.count + 1
	`, board, subType, value)

	return err
}

// Decrement decrements the count for a board/sub_type/value combination
func (p *SubscriptionStatsPostgres) Decrement(board, subType, value string) error {
	ctx := context.Background()
	pool := connections.Postgres()

	_, err := pool.Exec(ctx, `
		UPDATE subscription_stats
		SET count = GREATEST(count - 1, 0)
		WHERE board = $1 AND sub_type = $2 AND value = $3
	`, board, subType, value)

	return err
}

// IncrementBatch increments counts for multiple values
func (p *SubscriptionStatsPostgres) IncrementBatch(board, subType string, values []string) error {
	for _, value := range values {
		if err := p.Increment(board, subType, value); err != nil {
			return err
		}
	}
	return nil
}

// DecrementBatch decrements counts for multiple values
func (p *SubscriptionStatsPostgres) DecrementBatch(board, subType string, values []string) error {
	for _, value := range values {
		if err := p.Decrement(board, subType, value); err != nil {
			return err
		}
	}
	return nil
}

// ListByType returns all stats for a given sub_type, ordered by count desc
func (p *SubscriptionStatsPostgres) ListByType(subType string, limit int) ([]*SubscriptionStat, error) {
	ctx := context.Background()
	pool := connections.Postgres()

	var rows interface {
		Next() bool
		Scan(dest ...any) error
		Close()
	}
	var err error

	if limit > 0 {
		rows, err = pool.Query(ctx, `
			SELECT id, board, sub_type, value, count, updated_at
			FROM subscription_stats
			WHERE sub_type = $1 AND count > 0
			ORDER BY count DESC
			LIMIT $2
		`, subType, limit)
	} else {
		rows, err = pool.Query(ctx, `
			SELECT id, board, sub_type, value, count, updated_at
			FROM subscription_stats
			WHERE sub_type = $1 AND count > 0
			ORDER BY count DESC
		`, subType)
	}

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []*SubscriptionStat
	for rows.Next() {
		var stat SubscriptionStat
		err := rows.Scan(
			&stat.ID,
			&stat.Board,
			&stat.SubType,
			&stat.Value,
			&stat.Count,
			&stat.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		stats = append(stats, &stat)
	}

	return stats, nil
}

// ListByBoard returns all stats for a given board, ordered by count desc
func (p *SubscriptionStatsPostgres) ListByBoard(board string, limit int) ([]*SubscriptionStat, error) {
	ctx := context.Background()
	pool := connections.Postgres()

	var rows interface {
		Next() bool
		Scan(dest ...any) error
		Close()
	}
	var err error

	if limit > 0 {
		rows, err = pool.Query(ctx, `
			SELECT id, board, sub_type, value, count, updated_at
			FROM subscription_stats
			WHERE board = $1 AND count > 0
			ORDER BY count DESC
			LIMIT $2
		`, board, limit)
	} else {
		rows, err = pool.Query(ctx, `
			SELECT id, board, sub_type, value, count, updated_at
			FROM subscription_stats
			WHERE board = $1 AND count > 0
			ORDER BY count DESC
		`, board)
	}

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []*SubscriptionStat
	for rows.Next() {
		var stat SubscriptionStat
		err := rows.Scan(
			&stat.ID,
			&stat.Board,
			&stat.SubType,
			&stat.Value,
			&stat.Count,
			&stat.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		stats = append(stats, &stat)
	}

	return stats, nil
}

// ParseKeywordValues parses a keyword value and returns individual keywords
func ParseKeywordValues(value string) []string {
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
