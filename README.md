# wrds-dl

A terminal tool for browsing and downloading data from the [WRDS](https://wrds-www.wharton.upenn.edu/) PostgreSQL database. Comes with an interactive TUI for exploration and a CLI for scripted downloads. Output is Parquet or CSV — pure Go, no CGo, cross-platform.

## Features

- **TUI** — browse schemas and tables, inspect column metadata, trigger downloads without leaving the terminal
- **CLI** — scriptable `download` command with structured flags or raw SQL
- **`info` command** — inspect table metadata (columns, types, row count) from the command line or scripts
- **Parquet output** — streams rows via pgx and writes Parquet with ZSTD compression using parquet-go (pure Go)
- **CSV output** — streams rows to CSV via encoding/csv
- **Progress feedback** — live row count during large exports (CLI and TUI)
- **Dry-run mode** — preview the query, row count, and first 5 rows before committing to a download
- **Login flow** — interactive login screen with Duo 2FA support; saved credentials for one-press reconnect
- **Database switching** — browse and switch between WRDS databases from within the TUI
- **Standard auth** — reads from `PG*` environment variables, `~/.config/wrds-dl/credentials`, or `~/.pgpass`

## Installation

### Pre-built binaries (recommended)

Download the latest release from the [Releases page](https://github.com/louloulibs/wrds-download/releases):

| Platform | Binary |
|---|---|
| macOS (Apple Silicon) | `wrds-dl-darwin-arm64` |
| macOS (Intel) | `wrds-dl-darwin-amd64` |
| Linux x86-64 | `wrds-dl-linux-amd64` |
| Windows x86-64 | `wrds-dl-windows-amd64.exe` |

```sh
# macOS example
curl -L https://github.com/louloulibs/wrds-download/releases/latest/download/wrds-dl-darwin-arm64 \
  -o /usr/local/bin/wrds-dl
chmod +x /usr/local/bin/wrds-dl
```

### Build from source

Requires Go 1.25+. No CGo or C compiler needed.

```sh
git clone https://github.com/louloulibs/wrds-download
cd wrds-download
CGO_ENABLED=0 go build -ldflags="-s -w" -o wrds-dl .
mv wrds-dl /usr/local/bin/
```

## Authentication

WRDS uses Duo two-factor authentication. The TUI always starts on a login screen so you control when the connection (and Duo push) fires.

### Option 1: Environment variables

Set the standard PostgreSQL environment variables before running:

```sh
export PGUSER=your_username
export PGPASSWORD=your_password
```

Optional (defaults shown):

```sh
export PGHOST=wrds-pgdata.wharton.upenn.edu
export PGPORT=9737
export PGDATABASE=wrds
```

### Option 2: Saved credentials

On first login via the TUI, check "Save to ~/.config/wrds-dl/credentials". On subsequent launches, press `enter` on the "Login as ..." button. The credentials file is stored at `~/.config/wrds-dl/credentials` (or `$XDG_CONFIG_HOME/wrds-dl/credentials`) with `0600` permissions:

```
PGUSER=your_username
PGPASSWORD=your_password
PGDATABASE=wrds
```

### Option 3: ~/.pgpass

Standard PostgreSQL password file:

```
wrds-pgdata.wharton.upenn.edu:9737:*:your_username:your_password
```

## TUI

Launch the interactive browser:

```sh
wrds-dl tui
```

The TUI has three panes: **Schemas**, **Tables**, and **Preview** (column catalog with types, descriptions, and table stats).

### Keybindings

| Key | Action |
|---|---|
| `tab` / `shift+tab` | Cycle focus between panes |
| `right` / `l` | Drill into selected schema or table |
| `left` / `h` | Go back one pane |
| `d` | Open download dialog for the selected table |
| `b` | Switch database |
| `/` | Filter current list (schemas, tables, or columns) |
| `esc` | Cancel / dismiss overlay |
| `q` / `ctrl+c` | Quit |

### Download dialog

Press `d` on a selected table to open the download form:

| Field | Description |
|---|---|
| SELECT columns | Comma-separated column names, or `*` for all |
| WHERE clause | SQL filter without the `WHERE` keyword |
| LIMIT rows | Maximum number of rows to download (leave empty for no limit) |
| Output path | File path; defaults to `./schema_table.parquet` |
| Format | `parquet` or `csv` |

Navigate with `tab`/`shift+tab`, confirm with `enter` on the last field.

During download, the spinner shows a live row count updated every 10,000 rows.

## CLI

### Structured download

```sh
wrds-dl download \
  --schema crsp \
  --table dsf \
  --where "date >= '2020-01-01' AND date < '2021-01-01'" \
  --out crsp_dsf_2020.parquet
```

### Select specific columns

```sh
wrds-dl download \
  --schema comp \
  --table funda \
  --columns "gvkey,datadate,sale,at" \
  --out funda_subset.parquet
```

### Raw SQL

```sh
wrds-dl download \
  --query "SELECT permno, date, prc FROM crsp.dsf WHERE date > '2020-01-01'" \
  --out crsp_dsf.parquet
```

### CSV output

```sh
wrds-dl download \
  --schema comp \
  --table funda \
  --out funda.csv
```

Format is inferred from the output file extension (`.parquet` or `.csv`). Override with `--format`.

### Dry run

Preview what a download will do before committing:

```sh
wrds-dl download \
  --schema crsp \
  --table dsf \
  --where "date = '2020-01-02'" \
  --dry-run
```

This prints the SQL query, the row count, and the first 5 rows as a table. No `--out` flag is required for dry runs.

### All download flags

| Flag | Description |
|---|---|
| `--schema` | Schema name (e.g. `crsp`, `comp`) |
| `--table` | Table name (e.g. `dsf`, `funda`) |
| `-c`, `--columns` | Columns to select (comma-separated, default `*`) |
| `--where` | SQL `WHERE` clause, without the keyword |
| `--query` | Full SQL query — overrides `--schema`, `--table`, `--where`, `--columns` |
| `--out` | Output file path (required unless `--dry-run`) |
| `--format` | `parquet` or `csv` (inferred from extension if omitted) |
| `--limit` | Row limit, useful for testing (default: no limit) |
| `--dry-run` | Preview query, row count, and first 5 rows without downloading |

### Table info

Inspect table metadata without downloading data:

```sh
wrds-dl info --schema crsp --table dsf
```

Output:

```
crsp.dsf
  Daily Stock File
  ~245302893 rows, 47 GB

NAME        TYPE                          NULLABLE  DESCRIPTION
cusip       character varying(8)          YES       CUSIP - HISTORICAL
permno      double precision              YES       PERMNO
permco      double precision              YES       PERMCO
...
```

For machine-readable output (useful in scripts and coding assistants):

```sh
wrds-dl info --schema crsp --table dsf --json
```

## How it works

`wrds-dl` connects directly to the WRDS PostgreSQL server using [pgx](https://github.com/jackc/pgx). All operations — metadata browsing, column inspection, and data download — go through a single pooled connection (limited to 1 to avoid triggering multiple Duo 2FA prompts).

**Downloads** stream rows from Postgres and write them incrementally:
- **Parquet**: rows are batched (10,000 per row group) and written with ZSTD compression via [parquet-go](https://github.com/parquet-go/parquet-go). String columns use PLAIN encoding for broad compatibility (R, Python, Julia).
- **CSV**: rows are streamed directly to disk via Go's `encoding/csv`.

Progress is reported every 10,000 rows — printed to stderr on the CLI and shown in the TUI spinner overlay.

PostgreSQL types are mapped to Parquet types: `bool` → BOOLEAN, `int2/int4` → INT32, `int8` → INT64, `float4` → FLOAT, `float8` → DOUBLE, `date` → DATE, `timestamp/timestamptz` → TIMESTAMP (microseconds), `numeric` → STRING, `text/varchar/char` → STRING.

Schema and table names are quoted as PostgreSQL identifiers to prevent SQL injection. Column names from `--columns` are individually quoted.

## Project structure

```
wrds-download/
├── main.go                    # entrypoint
├── cmd/
│   ├── root.go                # cobra root command
│   ├── tui.go                 # `wrds-dl tui` — launches interactive browser
│   ├── download.go            # `wrds-dl download` — CLI download with --dry-run
│   └── info.go                # `wrds-dl info` — table metadata inspection
├── internal/
│   ├── db/
│   │   ├── client.go          # pgx pool, DSN construction, connection management
│   │   └── meta.go            # schema/table/column queries against pg_catalog
│   ├── export/
│   │   └── export.go          # Export() — pgx streaming → parquet-go / csv writer
│   ├── tui/
│   │   ├── app.go             # root Bubble Tea model, Update/View, pane navigation
│   │   ├── loginform.go       # login dialog with saved-credentials support
│   │   ├── dlform.go          # download dialog (columns, where, limit, output, format)
│   │   └── styles.go          # lipgloss styles and colors
│   └── config/
│       └── config.go          # credentials file read/write (~/.config/wrds-dl/)
└── .github/workflows/
    ├── ci.yml                 # CI: go vet, build, and test on push/PR
    └── release.yml            # Release: cross-compile 4 targets, attach to GitHub Release
```

## Dependencies

| Package | Purpose |
|---|---|
| [`jackc/pgx/v5`](https://github.com/jackc/pgx) | PostgreSQL driver — all queries and data streaming |
| [`parquet-go/parquet-go`](https://github.com/parquet-go/parquet-go) | Parquet file writing with ZSTD compression (pure Go) |
| [`charmbracelet/bubbletea`](https://github.com/charmbracelet/bubbletea) | TUI framework |
| [`charmbracelet/bubbles`](https://github.com/charmbracelet/bubbles) | List, text-input, spinner components |
| [`charmbracelet/lipgloss`](https://github.com/charmbracelet/lipgloss) | Terminal layout and styling |
| [`spf13/cobra`](https://github.com/spf13/cobra) | CLI commands and flags |
