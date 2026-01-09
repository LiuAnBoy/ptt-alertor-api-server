package article

import (
	"context"
	"database/sql"
	"strings"
	"time"

	log "github.com/Ptt-Alertor/logrus"
	"github.com/Ptt-Alertor/ptt-alertor/connections"
	"github.com/Ptt-Alertor/ptt-alertor/myutil"
)

// Postgres implements article.Driver interface
type Postgres struct{}

// Find retrieves an article and its comments from PostgreSQL
func (Postgres) Find(code string, a *Article) {
	ctx := context.Background()
	pool := connections.Postgres()

	// Query article
	var lastPushDT sql.NullTime
	err := pool.QueryRow(ctx, `
		SELECT code, id, title, link, date, author, board_name, push_sum, last_push_datetime,
		       positive_count, negative_count, neutral_count
		FROM articles
		WHERE code = $1
	`, code).Scan(
		&a.Code, &a.ID, &a.Title, &a.Link, &a.Date,
		&a.Author, &a.Board, &a.PushSum, &lastPushDT,
		&a.PositiveCount, &a.NegativeCount, &a.NeutralCount,
	)
	if err != nil {
		log.WithFields(log.Fields{
			"runtime": myutil.BasicRuntimeInfo(),
			"code":    code,
		}).WithError(err).Warn("PostgreSQL Find Article Failed")
		return
	}

	if lastPushDT.Valid {
		a.LastPushDateTime = lastPushDT.Time
	}

	// Query comments
	rows, err := pool.Query(ctx, `
		SELECT tag, user_id, content, datetime
		FROM comments
		WHERE article_code = $1
		ORDER BY id
	`, code)
	if err != nil {
		log.WithFields(log.Fields{
			"runtime": myutil.BasicRuntimeInfo(),
			"code":    code,
		}).WithError(err).Error("PostgreSQL Query Comments Failed")
		return
	}
	defer rows.Close()

	for rows.Next() {
		var c Comment
		var dt sql.NullTime

		err := rows.Scan(&c.Tag, &c.UserID, &c.Content, &dt)
		if err != nil {
			log.WithError(err).Error("PostgreSQL Scan Comment Failed")
			continue
		}
		if dt.Valid {
			c.DateTime = dt.Time
		}
		a.Comments = append(a.Comments, c)
	}
}

// Save stores an article and its comments to PostgreSQL using a transaction
func (Postgres) Save(a Article) error {
	ctx := context.Background()
	pool := connections.Postgres()

	tx, err := pool.Begin(ctx)
	if err != nil {
		log.WithField("runtime", myutil.BasicRuntimeInfo()).WithError(err).Error("PostgreSQL Begin Transaction Failed")
		return err
	}
	defer tx.Rollback(ctx)

	// Ensure board exists
	_, err = tx.Exec(ctx, `
		INSERT INTO boards (name) VALUES ($1)
		ON CONFLICT (name) DO NOTHING
	`, a.Board)
	if err != nil {
		log.WithField("runtime", myutil.BasicRuntimeInfo()).WithError(err).Error("PostgreSQL Insert Board Failed")
		return err
	}

	// Upsert article
	var lastPushDT interface{}
	if !a.LastPushDateTime.IsZero() {
		lastPushDT = a.LastPushDateTime
	}

	// Calculate comment statistics from Comments
	positiveCount, negativeCount, neutralCount := countCommentsByTag(a.Comments)

	_, err = tx.Exec(ctx, `
		INSERT INTO articles (code, id, title, link, date, author, board_name, push_sum, last_push_datetime,
		                      positive_count, negative_count, neutral_count)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (code) DO UPDATE SET
			id = EXCLUDED.id,
			title = EXCLUDED.title,
			link = EXCLUDED.link,
			date = EXCLUDED.date,
			author = EXCLUDED.author,
			push_sum = EXCLUDED.push_sum,
			last_push_datetime = EXCLUDED.last_push_datetime,
			positive_count = EXCLUDED.positive_count,
			negative_count = EXCLUDED.negative_count,
			neutral_count = EXCLUDED.neutral_count,
			updated_at = NOW()
	`, a.Code, a.ID, a.Title, a.Link, a.Date, a.Author, a.Board, a.PushSum, lastPushDT,
		positiveCount, negativeCount, neutralCount)
	if err != nil {
		log.WithField("runtime", myutil.BasicRuntimeInfo()).WithError(err).Error("PostgreSQL Upsert Article Failed")
		return err
	}

	// Delete old comments
	_, err = tx.Exec(ctx, `DELETE FROM comments WHERE article_code = $1`, a.Code)
	if err != nil {
		log.WithField("runtime", myutil.BasicRuntimeInfo()).WithError(err).Error("PostgreSQL Delete Comments Failed")
		return err
	}

	// Insert new comments
	for _, c := range a.Comments {
		var datetime interface{}
		if !c.DateTime.IsZero() {
			datetime = c.DateTime
		} else {
			datetime = time.Now()
		}

		_, err = tx.Exec(ctx, `
			INSERT INTO comments (article_code, tag, user_id, content, datetime)
			VALUES ($1, $2, $3, $4, $5)
		`, a.Code, c.Tag, c.UserID, c.Content, datetime)
		if err != nil {
			log.WithField("runtime", myutil.BasicRuntimeInfo()).WithError(err).Error("PostgreSQL Insert Comment Failed")
			return err
		}
	}

	err = tx.Commit(ctx)
	if err != nil {
		log.WithField("runtime", myutil.BasicRuntimeInfo()).WithError(err).Error("PostgreSQL Commit Transaction Failed")
		return err
	}

	return nil
}

// Delete removes an article from PostgreSQL (comments are deleted via CASCADE)
func (Postgres) Delete(code string) error {
	ctx := context.Background()
	pool := connections.Postgres()

	_, err := pool.Exec(ctx, `DELETE FROM articles WHERE code = $1`, code)
	if err != nil {
		log.WithFields(log.Fields{
			"runtime": myutil.BasicRuntimeInfo(),
			"code":    code,
		}).WithError(err).Error("PostgreSQL Delete Article Failed")
	}
	return err
}

// countCommentsByTag counts comments by their tag prefix (推/噓/→)
func countCommentsByTag(comments Comments) (positive, negative, neutral int) {
	for _, c := range comments {
		tag := strings.TrimSpace(c.Tag)
		if strings.HasPrefix(tag, "推") {
			positive++
		} else if strings.HasPrefix(tag, "噓") {
			negative++
		} else if strings.HasPrefix(tag, "→") {
			neutral++
		}
	}
	return
}
