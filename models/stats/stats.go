package stats

import (
	"context"
	"time"

	"github.com/Ptt-Alertor/ptt-alertor/connections"
	"github.com/Ptt-Alertor/ptt-alertor/models/counter"
	"github.com/gomodule/redigo/redis"
)

// ArticleStats represents article statistics
type ArticleStats struct {
	Total   int              `json:"total"`
	Today   int              `json:"today"`
	ByBoard []BoardCount     `json:"byBoard"`
	TopPush []TopPushArticle `json:"topPush"`
}

// BoardCount represents article count per board
type BoardCount struct {
	Board string `json:"board"`
	Count int    `json:"count"`
}

// PushCount represents the push/arrow/boo counts for an article
type PushCount struct {
	Positive int `json:"positive"`
	Neutral  int `json:"neutral"`
	Negative int `json:"negative"`
}

// TopPushArticle represents the article with highest push count
type TopPushArticle struct {
	Code   string    `json:"code"`
	Title  string    `json:"title"`
	Board  string    `json:"board"`
	Author string    `json:"author"`
	Push   PushCount `json:"push"`
}

// UserStats represents user statistics
type UserStats struct {
	Total    int         `json:"total"`
	Today    int         `json:"today"`
	ByRole   []RoleCount `json:"byRole"`
	Enabled  int         `json:"enabled"`
	Disabled int         `json:"disabled"`
}

// RoleCount represents user count per role
type RoleCount struct {
	Role  string `json:"role"`
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// SubscriptionStats represents subscription statistics
type SubscriptionStats struct {
	Total     int          `json:"total"`
	ByType    []TypeCount  `json:"byType"`
	TopBoards []BoardCount `json:"topBoards"`
}

// TypeCount represents subscription count per type
type TypeCount struct {
	Type  string `json:"type"`
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// BindingStats represents notification binding statistics
type BindingStats struct {
	Total      int            `json:"total"`
	TotalUsers int            `json:"totalUsers"`
	Ratio      float64        `json:"ratio"`
	ByService  []ServiceCount `json:"byService"`
}

// ServiceCount represents binding count per service
type ServiceCount struct {
	Service string  `json:"service"`
	Name    string  `json:"name"`
	Count   int     `json:"count"`
	Ratio   float64 `json:"ratio"`
}

// PTTAccountStats represents PTT account statistics
type PTTAccountStats struct {
	Total      int     `json:"total"`
	TotalUsers int     `json:"totalUsers"`
	Ratio      float64 `json:"ratio"`
}

// AlertStats represents alert statistics
type AlertStats struct {
	TotalSent int `json:"totalSent"`
}

// AdminInitResponse represents the admin/init API response
type AdminInitResponse struct {
	Articles      ArticleStats      `json:"articles"`
	Users         UserStats         `json:"users"`
	Subscriptions SubscriptionStats `json:"subscriptions"`
	Bindings      BindingStats      `json:"bindings"`
	PTTAccounts   PTTAccountStats   `json:"pttAccounts"`
	Alerts        AlertStats        `json:"alerts"`
}

// Service name mapping
var serviceNames = map[string]string{
	"telegram": "Telegram",
	"line":     "LINE",
	"discord":  "Discord",
}

// Role name mapping
var roleNames = map[string]string{
	"admin": "管理員",
	"vip":   "VIP",
	"user":  "一般用戶",
}

// Subscription type name mapping
var subTypeNames = map[string]string{
	"keyword": "關鍵字",
	"author":  "作者",
	"pushsum": "推文數",
}

// GetAdminStats retrieves all statistics for admin dashboard
func GetAdminStats() (*AdminInitResponse, error) {
	ctx := context.Background()
	pool := connections.Postgres()

	response := &AdminInitResponse{}

	// Get today's date start
	now := time.Now()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	// 1. Article statistics
	// Total articles
	pool.QueryRow(ctx, `SELECT COUNT(*) FROM articles`).Scan(&response.Articles.Total)

	// Today's articles
	pool.QueryRow(ctx, `SELECT COUNT(*) FROM articles WHERE created_at >= $1`, todayStart).Scan(&response.Articles.Today)

	// Articles by board (top 10)
	rows, _ := pool.Query(ctx, `
		SELECT board_name, COUNT(*) as count
		FROM articles
		GROUP BY board_name
		ORDER BY count DESC
		LIMIT 10
	`)
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var bc BoardCount
			rows.Scan(&bc.Board, &bc.Count)
			response.Articles.ByBoard = append(response.Articles.ByBoard, bc)
		}
	}

	// Top 5 push articles (ordered by total push count: positive + neutral + negative)
	topPushRows, _ := pool.Query(ctx, `
		SELECT code, title, board_name, author, positive_count, neutral_count, negative_count
		FROM articles
		ORDER BY (positive_count + neutral_count + negative_count) DESC
		LIMIT 5
	`)
	if topPushRows != nil {
		defer topPushRows.Close()
		for topPushRows.Next() {
			var tp TopPushArticle
			topPushRows.Scan(&tp.Code, &tp.Title, &tp.Board, &tp.Author, &tp.Push.Positive, &tp.Push.Neutral, &tp.Push.Negative)
			response.Articles.TopPush = append(response.Articles.TopPush, tp)
		}
	}

	// 2. User statistics
	// Total users
	pool.QueryRow(ctx, `SELECT COUNT(*) FROM users`).Scan(&response.Users.Total)

	// Today's users
	pool.QueryRow(ctx, `SELECT COUNT(*) FROM users WHERE created_at >= $1`, todayStart).Scan(&response.Users.Today)

	// Users by role
	roleRows, _ := pool.Query(ctx, `
		SELECT role, COUNT(*) as count
		FROM users
		GROUP BY role
		ORDER BY count DESC
	`)
	if roleRows != nil {
		defer roleRows.Close()
		for roleRows.Next() {
			var rc RoleCount
			roleRows.Scan(&rc.Role, &rc.Count)
			rc.Name = roleNames[rc.Role]
			if rc.Name == "" {
				rc.Name = rc.Role
			}
			response.Users.ByRole = append(response.Users.ByRole, rc)
		}
	}

	// Enabled/disabled users
	pool.QueryRow(ctx, `SELECT COUNT(*) FROM users WHERE enabled = true`).Scan(&response.Users.Enabled)
	pool.QueryRow(ctx, `SELECT COUNT(*) FROM users WHERE enabled = false`).Scan(&response.Users.Disabled)

	// 3. Subscription statistics
	// Total subscriptions
	pool.QueryRow(ctx, `SELECT COUNT(*) FROM subscriptions`).Scan(&response.Subscriptions.Total)

	// Subscriptions by type
	typeRows, _ := pool.Query(ctx, `
		SELECT sub_type, COUNT(*) as count
		FROM subscriptions
		GROUP BY sub_type
		ORDER BY count DESC
	`)
	if typeRows != nil {
		defer typeRows.Close()
		for typeRows.Next() {
			var tc TypeCount
			typeRows.Scan(&tc.Type, &tc.Count)
			tc.Name = subTypeNames[tc.Type]
			if tc.Name == "" {
				tc.Name = tc.Type
			}
			response.Subscriptions.ByType = append(response.Subscriptions.ByType, tc)
		}
	}

	// Top subscribed boards
	boardRows, _ := pool.Query(ctx, `
		SELECT board, COUNT(*) as count
		FROM subscriptions
		GROUP BY board
		ORDER BY count DESC
		LIMIT 10
	`)
	if boardRows != nil {
		defer boardRows.Close()
		for boardRows.Next() {
			var bc BoardCount
			boardRows.Scan(&bc.Board, &bc.Count)
			response.Subscriptions.TopBoards = append(response.Subscriptions.TopBoards, bc)
		}
	}

	// 4. Binding statistics
	totalUsers := response.Users.Total
	response.Bindings.TotalUsers = totalUsers

	// Total bindings (unique users with any binding)
	pool.QueryRow(ctx, `
		SELECT COUNT(DISTINCT user_id) FROM notification_bindings WHERE service_id != ''
	`).Scan(&response.Bindings.Total)

	if totalUsers > 0 {
		response.Bindings.Ratio = float64(response.Bindings.Total) / float64(totalUsers) * 100
	}

	// Bindings by service
	serviceRows, _ := pool.Query(ctx, `
		SELECT service, COUNT(*) as count
		FROM notification_bindings
		WHERE service_id != ''
		GROUP BY service
		ORDER BY count DESC
	`)
	if serviceRows != nil {
		defer serviceRows.Close()
		for serviceRows.Next() {
			var sc ServiceCount
			serviceRows.Scan(&sc.Service, &sc.Count)
			sc.Name = serviceNames[sc.Service]
			if sc.Name == "" {
				sc.Name = sc.Service
			}
			if totalUsers > 0 {
				sc.Ratio = float64(sc.Count) / float64(totalUsers) * 100
			}
			response.Bindings.ByService = append(response.Bindings.ByService, sc)
		}
	}

	// 5. PTT account statistics
	response.PTTAccounts.TotalUsers = totalUsers
	pool.QueryRow(ctx, `SELECT COUNT(*) FROM ptt_accounts`).Scan(&response.PTTAccounts.Total)
	if totalUsers > 0 {
		response.PTTAccounts.Ratio = float64(response.PTTAccounts.Total) / float64(totalUsers) * 100
	}

	// 6. Alert statistics (from Redis)
	alertCount, err := counter.Alert()
	if err == nil {
		response.Alerts.TotalSent = alertCount
	} else if err == redis.ErrNil {
		response.Alerts.TotalSent = 0
	}

	return response, nil
}
