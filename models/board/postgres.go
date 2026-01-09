package board

import (
	"context"
	"database/sql"

	log "github.com/Ptt-Alertor/logrus"
	"github.com/Ptt-Alertor/ptt-alertor/connections"
	"github.com/Ptt-Alertor/ptt-alertor/models/article"
	"github.com/Ptt-Alertor/ptt-alertor/myutil"
)

// Postgres implements board.Driver interface
type Postgres struct{}

// GetArticles retrieves all articles for a board from PostgreSQL
func (Postgres) GetArticles(boardName string) (articles article.Articles) {
	ctx := context.Background()
	pool := connections.Postgres()

	rows, err := pool.Query(ctx, `
		SELECT code, id, title, link, date, author, push_sum, last_push_datetime,
		       positive_count, negative_count, neutral_count
		FROM articles
		WHERE board_name = $1
		ORDER BY id DESC
	`, boardName)
	if err != nil {
		log.WithFields(log.Fields{
			"runtime": myutil.BasicRuntimeInfo(),
			"board":   boardName,
		}).WithError(err).Error("PostgreSQL GetArticles Failed")
		return
	}
	defer rows.Close()

	for rows.Next() {
		var a article.Article
		var lastPushDT sql.NullTime

		err := rows.Scan(
			&a.Code, &a.ID, &a.Title, &a.Link,
			&a.Date, &a.Author, &a.PushSum, &lastPushDT,
			&a.PositiveCount, &a.NegativeCount, &a.NeutralCount,
		)
		if err != nil {
			log.WithError(err).Error("PostgreSQL Scan Article Failed")
			continue
		}

		if lastPushDT.Valid {
			a.LastPushDateTime = lastPushDT.Time
		}
		a.Board = boardName
		articles = append(articles, a)
	}

	return articles
}

// Save stores articles for a board to PostgreSQL
func (Postgres) Save(boardName string, articles article.Articles) error {
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
		ON CONFLICT (name) DO UPDATE SET updated_at = NOW()
	`, boardName)
	if err != nil {
		log.WithField("runtime", myutil.BasicRuntimeInfo()).WithError(err).Error("PostgreSQL Insert Board Failed")
		return err
	}

	// Upsert each article
	for _, a := range articles {
		var lastPushDT interface{}
		if !a.LastPushDateTime.IsZero() {
			lastPushDT = a.LastPushDateTime
		}

		_, err = tx.Exec(ctx, `
			INSERT INTO articles (code, id, title, link, date, author, board_name, push_sum, last_push_datetime)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			ON CONFLICT (code) DO UPDATE SET
				id = EXCLUDED.id,
				title = EXCLUDED.title,
				link = EXCLUDED.link,
				date = EXCLUDED.date,
				author = EXCLUDED.author,
				push_sum = EXCLUDED.push_sum,
				last_push_datetime = EXCLUDED.last_push_datetime,
				updated_at = NOW()
		`, a.Code, a.ID, a.Title, a.Link, a.Date, a.Author, boardName, a.PushSum, lastPushDT)
		if err != nil {
			log.WithFields(log.Fields{
				"runtime": myutil.BasicRuntimeInfo(),
				"code":    a.Code,
			}).WithError(err).Error("PostgreSQL Upsert Article Failed")
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

// Delete removes all articles for a board from PostgreSQL
func (Postgres) Delete(boardName string) error {
	ctx := context.Background()
	pool := connections.Postgres()

	_, err := pool.Exec(ctx, `DELETE FROM articles WHERE board_name = $1`, boardName)
	if err != nil {
		log.WithFields(log.Fields{
			"runtime": myutil.BasicRuntimeInfo(),
			"board":   boardName,
		}).WithError(err).Error("PostgreSQL Delete Board Articles Failed")
	}
	return err
}
