# wrds-dl

A terminal tool for browsing and downloading data from the [WRDS](https://wrds-www.wharton.upenn.edu/) PostgreSQL database. Comes with an interactive TUI for exploration and a CLI for scripted downloads. Output is Parquet (via DuckDB) or CSV.

## Features

- **TUI** — browse schemas and tables, preview rows, trigger downloads without leaving the terminal
- **CLI** — scriptable `download` command with structured flags or raw SQL
- **Parquet output** — uses DuckDB's `postgres_scanner` for fast, efficient export with ZSTD compression
- **CSV output** — plain CSV alternative via the same DuckDB pipeline
- **Standard auth** — reads from `PG*` environment variables or `~/.pgpass`, no configuration file needed

## Installation

### Pre-built binaries (recommended)

Download the latest release from the [Releases page](https://github.com/eloualiche/wrds-download/releases):

| Platform | Binary |
|---|---|
| macOS (Apple Silicon) | `wrds-dl-darwin-arm64` |
| Linux x86-64 | `wrds-dl-linux-amd64` |

```sh
# macOS example
curl -L https://github.com/eloualiche/wrds-download/releases/latest/download/wrds-dl-darwin-arm64 \
  -o /usr/local/bin/wrds-dl
chmod +x /usr/local/bin/wrds-dl
```

### Build from source

Requires Go 1.21+, CGo, and a C++ compiler (`gcc-c++` on Linux, Xcode CLT on macOS).

```sh
git clone https://github.com/eloualiche/wrds-download
cd wrds-download
go build -o wrds-dl .
mv wrds-dl /usr/local/bin/
```

## Authentication

`wrds-dl` uses the standard PostgreSQL environment variables. Set them before running:

```sh
export PGHOST=wrds-pgdata.wharton.upenn.edu
export PGPORT=9737
export PGUSER=your_username
export PGPASSWORD=your_password
export PGDATABASE=your_username   # on WRDS, database name = username
```

Alternatively, store credentials in `~/.pgpass` (no `PGPASSWORD` needed):

```
wrds-pgdata.wharton.upenn.edu:9737:your_username:your_username:your_password
```

## TUI

Launch the interactive browser:

```sh
wrds-dl tui
```

```
┌─ WRDS ──────────────────────────────────────────────────────────┐
│ Schemas         │ Tables (crsp)      │ Preview: crsp.dsf         │
│ ─────────────   │ ─────────────────  │ ──────────────────────    │
│ > crsp          │ > dsf              │ permno  date       prc    │
│   comp          │   mse              │ 10001   2020-01-02 45.23  │
│   ibes          │   ccm_final        │ 10001   2020-01-03 47.11  │
│   optionm       │   ...              │ ...                       │
│   ...           │                    │ ~2.1M rows                │
│                 │                    │                           │
│ [tab] switch pane  [d] download  [/] filter  [q] quit            │
└─────────────────────────────────────────────────────────────────┘
```

### Keybindings

| Key | Action |
|---|---|
| `tab` / `shift+tab` | Cycle focus between panes |
| `enter` | Drill into schema or table |
| `d` | Open download dialog for the selected table |
| `/` | Filter list |
| `esc` | Cancel / dismiss |
| `q` / `ctrl+c` | Quit |

In the download dialog, `tab` moves between fields and `enter` on the last field confirms.

## CLI

### Structured download

```sh
wrds-dl download \
  --schema crsp \
  --table dsf \
  --where "date >= '2020-01-01' AND date < '2021-01-01'" \
  --out crsp_dsf_2020.parquet
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

Format is inferred from the output file extension (`.parquet` → Parquet, `.csv` → CSV). Override with `--format`.

### Flags

| Flag | Description |
|---|---|
| `--schema` | Schema name (e.g. `crsp`, `comp`) |
| `--table` | Table name (e.g. `dsf`, `funda`) |
| `--where` | SQL `WHERE` clause, without the keyword (e.g. `date > '2020-01-01'`) |
| `--query` | Full SQL query — overrides `--schema`, `--table`, `--where` |
| `--out` | Output file path (required) |
| `--format` | `parquet` or `csv` (inferred from extension if omitted) |
| `--limit` | Row limit, useful for testing (default: no limit) |

## How it works

- **Metadata** (schema/table/column listing, row preview) uses a `pgx` connection pool talking directly to the WRDS PostgreSQL server.
- **Downloads** use [DuckDB](https://duckdb.org/) with the `postgres_scanner` extension. DuckDB attaches to WRDS as a read-only source and streams data directly into Parquet or CSV without loading it all into memory first.

## Dependencies

| Package | Purpose |
|---|---|
| `charmbracelet/bubbletea` | TUI framework |
| `charmbracelet/bubbles` | List, table, text-input, spinner components |
| `charmbracelet/lipgloss` | Layout and styling |
| `jackc/pgx/v5` | PostgreSQL driver for metadata and preview |
| `spf13/cobra` | CLI commands and flags |
| `marcboeker/go-duckdb` | Parquet/CSV export via `postgres_scanner` |
