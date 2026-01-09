package jobs

import (
	"context"
	"time"

	log "github.com/Ptt-Alertor/logrus"
	"github.com/Ptt-Alertor/ptt-alertor/connections"
	"github.com/Ptt-Alertor/ptt-alertor/models/article"
	"github.com/Ptt-Alertor/ptt-alertor/myutil"
	"github.com/Ptt-Alertor/ptt-alertor/ptt/web"
)

// CommentAggregator aggregates comment statistics for recent articles
type CommentAggregator struct {
	duration time.Duration
}

// NewCommentAggregator creates a new CommentAggregator
func NewCommentAggregator() *CommentAggregator {
	return &CommentAggregator{
		duration: 500 * time.Millisecond, // delay between requests to avoid rate limiting
	}
}

// Run executes the comment aggregation job
func (ca CommentAggregator) Run() {
	log.Info("Comment Aggregator Started")

	articles := ca.getRecentArticles()
	if len(articles) == 0 {
		log.Info("Comment Aggregator: No recent articles found")
		return
	}

	log.WithField("count", len(articles)).Info("Comment Aggregator: Processing articles")

	updated := 0
	for _, a := range articles {
		time.Sleep(ca.duration)

		fetched, err := web.FetchArticle(a.Board, a.Code)
		if err != nil {
			if _, ok := err.(web.URLNotFoundError); ok {
				log.WithFields(log.Fields{
					"board": a.Board,
					"code":  a.Code,
				}).Debug("Comment Aggregator: Article not found, skipping")
				continue
			}
			log.WithFields(log.Fields{
				"board": a.Board,
				"code":  a.Code,
			}).WithError(err).Warn("Comment Aggregator: Failed to fetch article")
			continue
		}

		// Update article with fetched comments
		fetched.ID = a.ID
		fetched.Date = a.Date
		fetched.Author = a.Author
		fetched.PushSum = a.PushSum

		postgres := article.Postgres{}
		if err := postgres.Save(fetched); err != nil {
			log.WithFields(log.Fields{
				"board": a.Board,
				"code":  a.Code,
			}).WithError(err).Warn("Comment Aggregator: Failed to save article")
			continue
		}

		updated++
	}

	log.WithFields(log.Fields{
		"total":   len(articles),
		"updated": updated,
	}).Info("Comment Aggregator Completed")
}

// getRecentArticles retrieves articles created within the last 1 day
func (ca CommentAggregator) getRecentArticles() []article.Article {
	ctx := context.Background()
	pool := connections.Postgres()

	rows, err := pool.Query(ctx, `
		SELECT code, id, title, link, date, author, board_name, push_sum
		FROM articles
		WHERE created_at > NOW() - INTERVAL '1 day'
		ORDER BY created_at DESC
	`)
	if err != nil {
		log.WithField("runtime", myutil.BasicRuntimeInfo()).WithError(err).Error("Comment Aggregator: Query Failed")
		return nil
	}
	defer rows.Close()

	var articles []article.Article
	for rows.Next() {
		var a article.Article
		err := rows.Scan(
			&a.Code, &a.ID, &a.Title, &a.Link,
			&a.Date, &a.Author, &a.Board, &a.PushSum,
		)
		if err != nil {
			log.WithError(err).Error("Comment Aggregator: Scan Failed")
			continue
		}
		articles = append(articles, a)
	}

	return articles
}
