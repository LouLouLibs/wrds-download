package db

import (
	"context"
	"fmt"
	"strings"
)

// Schema represents a PostgreSQL schema.
type Schema struct {
	Name string
}

// Table represents a PostgreSQL table within a schema.
type Table struct {
	Schema string
	Name   string
}

// Column represents a column in a table.
type Column struct {
	Name     string
	DataType string
}

// PreviewResult holds sample rows and row count for a table.
type PreviewResult struct {
	Columns []string
	Rows    [][]string
	Total   int64 // estimated row count
}

// Schemas returns all non-system schemas sorted by name.
func (c *Client) Schemas(ctx context.Context) ([]Schema, error) {
	rows, err := c.Pool.Query(ctx, `
		SELECT schema_name
		FROM information_schema.schemata
		WHERE schema_name NOT IN ('pg_catalog', 'information_schema', 'pg_toast',
		                          'pg_temp_1', 'pg_toast_temp_1')
		  AND schema_name NOT LIKE 'pg_%'
		ORDER BY schema_name
	`)
	if err != nil {
		return nil, fmt.Errorf("schemas query: %w", err)
	}
	defer rows.Close()

	var schemas []Schema
	for rows.Next() {
		var s Schema
		if err := rows.Scan(&s.Name); err != nil {
			return nil, err
		}
		schemas = append(schemas, s)
	}
	return schemas, rows.Err()
}

// Tables returns all tables in the given schema.
func (c *Client) Tables(ctx context.Context, schema string) ([]Table, error) {
	rows, err := c.Pool.Query(ctx, `
		SELECT table_name
		FROM information_schema.tables
		WHERE table_schema = $1
		  AND table_type IN ('BASE TABLE', 'VIEW')
		ORDER BY table_name
	`, schema)
	if err != nil {
		return nil, fmt.Errorf("tables query: %w", err)
	}
	defer rows.Close()

	var tables []Table
	for rows.Next() {
		var t Table
		t.Schema = schema
		if err := rows.Scan(&t.Name); err != nil {
			return nil, err
		}
		tables = append(tables, t)
	}
	return tables, rows.Err()
}

// Columns returns column metadata for the given table.
func (c *Client) Columns(ctx context.Context, schema, table string) ([]Column, error) {
	rows, err := c.Pool.Query(ctx, `
		SELECT column_name, data_type
		FROM information_schema.columns
		WHERE table_schema = $1 AND table_name = $2
		ORDER BY ordinal_position
	`, schema, table)
	if err != nil {
		return nil, fmt.Errorf("columns query: %w", err)
	}
	defer rows.Close()

	var cols []Column
	for rows.Next() {
		var col Column
		if err := rows.Scan(&col.Name, &col.DataType); err != nil {
			return nil, err
		}
		cols = append(cols, col)
	}
	return cols, rows.Err()
}

// Preview fetches the first `limit` rows and an estimated row count.
func (c *Client) Preview(ctx context.Context, schema, table string, limit int) (*PreviewResult, error) {
	if limit <= 0 {
		limit = 50
	}

	qualified := fmt.Sprintf("%s.%s", quoteIdent(schema), quoteIdent(table))

	// Estimated count via pg stats (fast).
	var total int64
	_ = c.Pool.QueryRow(ctx, `
		SELECT reltuples::bigint
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = $1 AND c.relname = $2
	`, schema, table).Scan(&total)

	rows, err := c.Pool.Query(ctx, fmt.Sprintf("SELECT * FROM %s LIMIT %d", qualified, limit))
	if err != nil {
		return nil, fmt.Errorf("preview query: %w", err)
	}
	defer rows.Close()

	fieldDescs := rows.FieldDescriptions()
	cols := make([]string, len(fieldDescs))
	for i, fd := range fieldDescs {
		cols[i] = string(fd.Name)
	}

	var result PreviewResult
	result.Columns = cols
	result.Total = total

	for rows.Next() {
		vals, err := rows.Values()
		if err != nil {
			return nil, err
		}
		row := make([]string, len(vals))
		for i, v := range vals {
			if v == nil {
				row[i] = "NULL"
			} else {
				row[i] = fmt.Sprintf("%v", v)
			}
		}
		result.Rows = append(result.Rows, row)
	}
	return &result, rows.Err()
}

func quoteIdent(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}
