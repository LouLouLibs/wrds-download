package db

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Client wraps a pgx connection pool.
type Client struct {
	Pool *pgxpool.Pool
}

// DSNFromEnv builds a PostgreSQL DSN from standard PG environment variables.
func DSNFromEnv() string {
	host := getenv("PGHOST", "wrds-pgdata.wharton.upenn.edu")
	port := getenv("PGPORT", "9737")
	user := getenv("PGUSER", "")
	password := getenv("PGPASSWORD", "")
	database := getenv("PGDATABASE", user) // WRDS default db = username

	if password != "" {
		return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=require",
			host, port, user, password, database)
	}
	return fmt.Sprintf("host=%s port=%s user=%s dbname=%s sslmode=require",
		host, port, user, database)
}

// PortFromEnv returns the port as an integer (for DuckDB attach).
func PortFromEnv() int {
	p := getenv("PGPORT", "9737")
	n, _ := strconv.Atoi(p)
	if n == 0 {
		n = 9737
	}
	return n
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// New creates and pings a pgx pool using DSNFromEnv.
func New(ctx context.Context) (*Client, error) {
	dsn := DSNFromEnv()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("pgxpool.New: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	return &Client{Pool: pool}, nil
}

// Close releases the pool.
func (c *Client) Close() {
	c.Pool.Close()
}
