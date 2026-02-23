# wrds-dl

A terminal tool for browsing and downloading data from the [WRDS](https://wrds-www.wharton.upenn.edu/) PostgreSQL database. Output is Parquet or CSV.

Two implementations with the same CLI interface — pick whichever fits your environment:

| | [Go](go/) | [Python](python/) |
|---|---|---|
| **Install** | Pre-built binary (~19 MB) | `uv tool install` / `uv run` |
| **TUI browser** | Yes | No |
| **CLI commands** | `download`, `info` | `download`, `info` |
| **Dependencies** | None (static binary) | Python 3.10+, `uv` |
| **Best for** | Interactive exploration, offline use | HPC clusters, CI, quick installs |

## Quick start

### Go

```sh
# Download binary (macOS Apple Silicon example)
curl -L https://github.com/louloulibs/wrds-download/releases/latest/download/wrds-dl-darwin-arm64 \
  -o /usr/local/bin/wrds-dl
chmod +x /usr/local/bin/wrds-dl
```

### Python

```sh
# Install as a uv tool
uv tool install wrds-dl --from ./python

# Or run directly
cd python && uv run wrds-dl --help
```

## CLI (both implementations)

```sh
# CRSP monthly stock file — prices and returns for 2020
wrds-dl download --schema crsp --table msf \
  --columns "permno,date,prc,ret,shrout" \
  --where "date >= '2020-01-01' AND date < '2021-01-01'" \
  --out crsp_msf_2020.parquet

# Inspect table metadata before downloading
wrds-dl info --schema crsp --table msf

# Dry run — preview query, row count, and sample rows
wrds-dl download --schema crsp --table msf \
  --where "date = '2020-01-31'" --dry-run

# CRSP daily stock file
wrds-dl download --schema crsp --table dsf \
  --where "date >= '2020-01-01' AND date < '2021-01-01'" \
  --out crsp_dsf_2020.parquet

# Select specific columns from Compustat
wrds-dl download --schema comp --table funda \
  --columns "gvkey,datadate,sale,at" \
  --out funda_subset.parquet

# Raw SQL
wrds-dl download \
  --query "SELECT permno, date, prc FROM crsp.msf WHERE date >= '2015-01-01'" \
  --out crsp_msf_2015_onwards.parquet

# CSV output
wrds-dl download --schema crsp --table msf \
  --columns "permno,date,ret" --limit 1000 \
  --out crsp_msf_sample.csv
```

## Claude Code skill

Bundled [Claude Code](https://claude.com/claude-code) skills let you download WRDS data using natural language:

```
/wrds-download CRSP daily stock data for 2020
```

Two variants available:
- [`claude-skill-wrds-download/`](claude-skill-wrds-download/) — uses the Go binary
- [`claude-skill-wrds-download-py/`](claude-skill-wrds-download-py/) — uses the Python CLI (no binary needed)

Install by copying the skill into your skills directory:

```sh
cp -r claude-skill-wrds-download-py ~/.claude/skills/wrds-download
```

## Authentication

WRDS uses Duo two-factor authentication. Configure credentials before using the CLI.

### Option 1: Environment variables

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

Store credentials at `~/.config/wrds-dl/credentials` (or `$XDG_CONFIG_HOME/wrds-dl/credentials`) with `0600` permissions:

```
PGUSER=your_username
PGPASSWORD=your_password
PGDATABASE=wrds
```

The Go TUI can save these automatically on first login.

### Option 3: ~/.pgpass

Standard PostgreSQL password file:

```
wrds-pgdata.wharton.upenn.edu:9737:*:your_username:your_password
```

## Project structure

```
wrds-download/
├── go/                             # Go implementation (TUI + CLI)
│   ├── main.go
│   ├── cmd/                        # download, info, tui commands
│   └── internal/                   # db, export, tui, config modules
├── python/                         # Python implementation (CLI only)
│   ├── pyproject.toml
│   └── src/wrds_dl/                # cli, db, export, config modules
├── claude-skill-wrds-download/     # Claude skill (Go binary)
├── claude-skill-wrds-download-py/  # Claude skill (Python/uv)
└── .github/workflows/              # CI for both implementations
```
