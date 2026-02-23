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

// ColumnMeta holds catalog metadata about a single column.
type ColumnMeta struct {
	Name        string
	DataType    string
	Nullable    bool
	Description string // from pg_description (WRDS variable label)
}

// TableMeta holds catalog metadata for a table (no data scan required).
type TableMeta struct {
	Schema   string
	Table    string
	Comment  string // table-level comment from pg_description
	RowCount int64  // estimated from pg_class.reltuples
	Size     string // human-readable, from pg_size_pretty
	Columns  []ColumnMeta
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

// TableMeta fetches catalog metadata for a table: column info with
// descriptions, estimated row count, and table size. All queries hit
// pg_catalog only — no table data is scanned.
func (c *Client) TableMeta(ctx context.Context, schema, table string) (*TableMeta, error) {
	meta := &TableMeta{Schema: schema, Table: table}

	// Table-level stats (best effort — some may require permissions).
	_ = c.Pool.QueryRow(ctx, `
		SELECT c.reltuples::bigint,
		       COALESCE(pg_size_pretty(pg_total_relation_size(c.oid)), ''),
		       COALESCE(obj_description(c.oid), '')
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = $1 AND c.relname = $2
	`, schema, table).Scan(&meta.RowCount, &meta.Size, &meta.Comment)

	// Column metadata with descriptions from pg_description.
	rows, err := c.Pool.Query(ctx, `
		SELECT a.attname,
		       pg_catalog.format_type(a.atttypid, a.atttypmod),
		       NOT a.attnotnull,
		       COALESCE(d.description, '')
		FROM pg_attribute a
		JOIN pg_class c ON a.attrelid = c.oid
		JOIN pg_namespace n ON c.relnamespace = n.oid
		LEFT JOIN pg_description d ON d.objoid = c.oid AND d.objsubid = a.attnum
		WHERE n.nspname = $1 AND c.relname = $2
		  AND a.attnum > 0 AND NOT a.attisdropped
		ORDER BY a.attnum
	`, schema, table)
	if err != nil {
		return nil, fmt.Errorf("table meta: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var col ColumnMeta
		if err := rows.Scan(&col.Name, &col.DataType, &col.Nullable, &col.Description); err != nil {
			return nil, err
		}
		meta.Columns = append(meta.Columns, col)
	}
	return meta, rows.Err()
}

// Preview fetches the first `limit` rows and an estimated row count.
func (c *Client) Preview(ctx context.Context, schema, table string, limit int) (*PreviewResult, error) {
	if limit <= 0 {
		limit = 50
	}

	qualified := fmt.Sprintf("%s.%s", QuoteIdent(schema), QuoteIdent(table))

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

// QuoteIdent quotes a PostgreSQL identifier (schema, table, column name)
// to prevent SQL injection.
func QuoteIdent(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}
