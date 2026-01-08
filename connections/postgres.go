package connections

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	pgPool     *pgxpool.Pool
	pgPoolOnce sync.Once
)

// Postgres returns the PostgreSQL connection pool
func Postgres() *pgxpool.Pool {
	pgPoolOnce.Do(func() {
		host := os.Getenv("PG_HOST")
		port := os.Getenv("PG_PORT")
		user := os.Getenv("PG_USER")
		password := os.Getenv("PG_PASSWORD")
		database := os.Getenv("PG_DATABASE")
		poolMax := os.Getenv("PG_POOL_MAX")

		if poolMax == "" {
			poolMax = "10"
		}

		connString := fmt.Sprintf(
			"postgres://%s:%s@%s:%s/%s?pool_max_conns=%s",
			user, password, host, port, database, poolMax,
		)

		var err error
		pgPool, err = pgxpool.New(context.Background(), connString)
		if err != nil {
			panic(fmt.Sprintf("Unable to connect to PostgreSQL: %v", err))
		}
	})
	return pgPool
}

// ClosePostgres closes the PostgreSQL connection pool
func ClosePostgres() {
	if pgPool != nil {
		pgPool.Close()
	}
}
