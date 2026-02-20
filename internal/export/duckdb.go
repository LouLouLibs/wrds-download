package export

import (
	"database/sql"
	"fmt"
	"os"
	"strings"

	_ "github.com/marcboeker/go-duckdb"
)

// Options controls the export behaviour.
type Options struct {
	Format string // "parquet" or "csv"
}

// Export runs query against the WRDS PostgreSQL instance and writes output to outPath.
// Format is determined by opts.Format (default: parquet).
func Export(query, outPath string, opts Options) error {
	format := strings.ToLower(opts.Format)
	if format == "" {
		if strings.HasSuffix(strings.ToLower(outPath), ".csv") {
			format = "csv"
		} else {
			format = "parquet"
		}
	}

	db, err := sql.Open("duckdb", "")
	if err != nil {
		return fmt.Errorf("open duckdb: %w", err)
	}
	defer db.Close()

	// Install and load postgres extension.
	for _, stmt := range []string{
		"INSTALL postgres;",
		"LOAD postgres;",
	} {
		if _, err := db.Exec(stmt); err != nil {
			// Ignore "already installed" errors.
			if !strings.Contains(err.Error(), "already") {
				return fmt.Errorf("%s: %w", stmt, err)
			}
		}
	}

	// Build the ATTACH string from env.
	attachDSN := buildAttachDSN()
	attach := fmt.Sprintf("ATTACH '%s' AS wrds (TYPE POSTGRES, READ_ONLY);", attachDSN)
	if _, err := db.Exec(attach); err != nil {
		return fmt.Errorf("attach: %w", err)
	}

	// Wrap query in a COPY statement.
	var copySQL string
	switch format {
	case "csv":
		copySQL = fmt.Sprintf("COPY (%s) TO '%s' (FORMAT CSV, HEADER true);", query, outPath)
	default:
		copySQL = fmt.Sprintf("COPY (%s) TO '%s' (FORMAT PARQUET, COMPRESSION ZSTD);", query, outPath)
	}

	if _, err := db.Exec(copySQL); err != nil {
		return fmt.Errorf("copy: %w", err)
	}
	return nil
}

// buildAttachDSN builds the postgres attach DSN string from standard PG env vars.
func buildAttachDSN() string {
	host := getenv("PGHOST", "wrds-pgdata.wharton.upenn.edu")
	port := getenv("PGPORT", "9737")
	user := getenv("PGUSER", "")
	password := getenv("PGPASSWORD", "")
	dbname := getenv("PGDATABASE", user)

	// DuckDB postgres attach DSN format.
	parts := []string{
		"host=" + host,
		"port=" + port,
		"dbname=" + dbname,
		"sslmode=require",
	}
	if user != "" {
		parts = append(parts, "user="+user)
	}
	if password != "" {
		parts = append(parts, "password="+password)
	}
	return strings.Join(parts, " ")
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
