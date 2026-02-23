package db

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNoUser is returned when PGUSER is not set.
var ErrNoUser = errors.New("PGUSER not set")

// Client wraps a pgx connection pool.
type Client struct {
	Pool *pgxpool.Pool
}

// DSNFromEnv builds a PostgreSQL DSN from standard PG environment variables.
// Returns ("", ErrNoUser) if PGUSER is empty.
func DSNFromEnv() (string, error) {
	host := getenv("PGHOST", "wrds-pgdata.wharton.upenn.edu")
	port := getenv("PGPORT", "9737")
	user := getenv("PGUSER", "")
	password := getenv("PGPASSWORD", "")
	database := getenv("PGDATABASE", "wrds")

	if user == "" {
		return "", ErrNoUser
	}

	dsn := fmt.Sprintf("host=%s port=%s user=%s sslmode=require", host, port, user)
	if password != "" {
		dsn += fmt.Sprintf(" password=%s", password)
	}
	if database != "" {
		dsn += fmt.Sprintf(" dbname=%s", database)
	}
	return dsn, nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// New creates and pings a pgx pool using DSNFromEnv.
// The pool is limited to a single connection to avoid triggering
// multiple authentication prompts (e.g. Duo 2FA on WRDS).
func New(ctx context.Context) (*Client, error) {
	dsn, err := DSNFromEnv()
	if err != nil {
		return nil, err
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse dsn: %w", err)
	}
	cfg.MaxConns = 1
	cfg.MinConns = 0
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("pgxpool.New: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	return &Client{Pool: pool}, nil
}

// NewWithCredentials sets PGUSER/PGPASSWORD/PGDATABASE env vars then creates and pings a pool.
func NewWithCredentials(ctx context.Context, user, password, database string) (*Client, error) {
	os.Setenv("PGUSER", user)
	os.Setenv("PGPASSWORD", password)
	if database != "" {
		os.Setenv("PGDATABASE", database)
	}
	return New(ctx)
}

// Databases returns the list of connectable databases.
func (c *Client) Databases(ctx context.Context) ([]string, error) {
	rows, err := c.Pool.Query(ctx,
		"SELECT datname FROM pg_database WHERE datallowconn = true ORDER BY datname")
	if err != nil {
		return nil, fmt.Errorf("databases query: %w", err)
	}
	defer rows.Close()

	var dbs []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		dbs = append(dbs, name)
	}
	return dbs, rows.Err()
}

// Close releases the pool.
func (c *Client) Close() {
	c.Pool.Close()
}
