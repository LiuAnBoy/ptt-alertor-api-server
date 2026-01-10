package top

import (
	"context"
	"time"

	"github.com/Ptt-Alertor/ptt-alertor/connections"
)

// Stat represents a subscription statistic entry
type Stat struct {
	ID        int       `json:"id"`
	Board     string    `json:"board"`
	SubType   string    `json:"sub_type"`
	Value     string    `json:"value"`
	Count     int       `json:"count"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Postgres is the PostgreSQL repository for subscription stats
type Postgres struct{}

// Increment increments the count for a board/sub_type/value combination
func (p *Postgres) Increment(board, subType, value string) error {
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
func (p *Postgres) Decrement(board, subType, value string) error {
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
func (p *Postgres) IncrementBatch(board, subType string, values []string) error {
	for _, value := range values {
		if err := p.Increment(board, subType, value); err != nil {
			return err
		}
	}
	return nil
}

// DecrementBatch decrements counts for multiple values
func (p *Postgres) DecrementBatch(board, subType string, values []string) error {
	for _, value := range values {
		if err := p.Decrement(board, subType, value); err != nil {
			return err
		}
	}
	return nil
}

// ListByType returns all stats for a given sub_type, ordered by count desc
func (p *Postgres) ListByType(subType string, limit int) ([]*Stat, error) {
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

	var stats []*Stat
	for rows.Next() {
		var stat Stat
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
func (p *Postgres) ListByBoard(board string, limit int) ([]*Stat, error) {
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

	var stats []*Stat
	for rows.Next() {
		var stat Stat
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
