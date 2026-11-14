# wrds-dl (Go)

Full-featured terminal tool for browsing and downloading data from the [WRDS](https://wrds-www.wharton.upenn.edu/) PostgreSQL database. Includes an interactive TUI for exploration and a CLI for scripted downloads. Output is Parquet or CSV — pure Go, no CGo, cross-platform.

## Features

- **TUI** — browse schemas and tables, inspect column metadata, trigger downloads without leaving the terminal
- **CLI** — scriptable `download` command with structured flags or raw SQL
- **`info` command** — inspect table metadata (columns, types, row count) from the command line or scripts
- **Parquet output** — streams rows via pgx and writes Parquet with ZSTD compression through Apache Arrow (arrow-go, pure Go)
- **CSV output** — streams rows to CSV via encoding/csv
- **Progress feedback** — live row count during large exports (CLI and TUI)
- **Dry-run mode** — preview the query, row count, and first 5 rows before committing to a download
- **Login flow** — interactive login screen with Duo 2FA support; saved credentials for one-press reconnect
- **Database switching** — browse and switch between WRDS databases from within the TUI

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
cd go
CGO_ENABLED=0 go build -ldflags="-s -w" -o wrds-dl .
mv wrds-dl /usr/local/bin/
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

## CLI

### CRSP monthly stock file

```sh
# Inspect the table first
wrds-dl info --schema crsp --table msf

# Download prices and returns for 2020
wrds-dl download \
  --schema crsp \
  --table msf \
  --columns "permno,date,prc,ret,shrout" \
  --where "date >= '2020-01-01' AND date < '2021-01-01'" \
  --out crsp_msf_2020.parquet

# Dry run — check row count before committing
wrds-dl download \
  --schema crsp \
  --table msf \
  --where "date = '2020-01-31'" \
  --dry-run
```

### CRSP daily stock file

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
  --query "SELECT permno, date, prc FROM crsp.msf WHERE date >= '2015-01-01'" \
  --out crsp_msf_2015_onwards.parquet
```

### CSV output

```sh
wrds-dl download \
  --schema crsp \
  --table msf \
  --columns "permno,date,ret" --limit 1000 \
  --out crsp_msf_sample.csv
```

Format is inferred from the output file extension (`.parquet` or `.csv`). Override with `--format`.

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

```sh
wrds-dl info --schema crsp --table dsf
```

For machine-readable output:

```sh
wrds-dl info --schema crsp --table dsf --json
```

## How it works

`wrds-dl` connects directly to the WRDS PostgreSQL server using [pgx](https://github.com/jackc/pgx). All operations go through a single pooled connection (limited to 1 to avoid triggering multiple Duo 2FA prompts).

**Downloads** stream rows from Postgres and write them incrementally:
- **Parquet**: rows are built into Arrow record batches (10,000 rows per row group) and written with ZSTD compression via [arrow-go](https://github.com/apache/arrow-go)'s `pqarrow`. Type mapping:

  | Postgres | Parquet |
  |---|---|
  | `bool`, `int2`/`int4`, `int8`, `float4`, `float8` | BOOLEAN, INT32, INT64, FLOAT, DOUBLE |
  | `numeric(p,s)`, p ≤ 16 | exact `DECIMAL(p,s)` stored as INT32/INT64 (`NaN` → NULL) |
  | `numeric` (unconstrained — `sum`/`avg` and most WRDS columns) or p > 16 | DOUBLE (`NaN`/`±Infinity` kept) |
  | `date`, `time`, `timestamp`, `timestamptz` | DATE, TIME(µs), TIMESTAMP(µs) local / UTC-adjusted |
  | everything else (`text`, `varchar`, `json`, `uuid`, arrays, `interval`, …) | STRING holding the Postgres text output |

  Postgres `±infinity` dates and timestamps are written as the extreme values DuckDB uses for them. The DECIMAL limits are for Julia's Parquet2.jl, which reads DECIMAL into 16-digit `Dec64` and mis-reads negative byte-array decimals.
- **CSV**: rows are streamed directly to disk via Go's `encoding/csv`.

## Dependencies

| Package | Purpose |
|---|---|
| [`jackc/pgx/v5`](https://github.com/jackc/pgx) | PostgreSQL driver — all queries and data streaming |
| [`apache/arrow-go`](https://github.com/apache/arrow-go) | Arrow record batches and Parquet writing (`pqarrow`) with ZSTD compression (pure Go) |
| [`charmbracelet/bubbletea`](https://github.com/charmbracelet/bubbletea) | TUI framework |
| [`charmbracelet/bubbles`](https://github.com/charmbracelet/bubbles) | List, text-input, spinner components |
| [`charmbracelet/lipgloss`](https://github.com/charmbracelet/lipgloss) | Terminal layout and styling |
| [`spf13/cobra`](https://github.com/spf13/cobra) | CLI commands and flags |
