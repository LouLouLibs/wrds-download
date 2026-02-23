# wrds-dl (Python)

Lightweight CLI for downloading data from the [WRDS](https://wrds-www.wharton.upenn.edu/) PostgreSQL database. No binary to install — runs anywhere Python and [`uv`](https://docs.astral.sh/uv/) are available.

Same CLI interface as the [Go version](../go/) (`download` and `info` commands with identical flags), but without the TUI.

## Installation

### With uv (recommended)

```sh
# Install as a tool (available system-wide)
uv tool install wrds-dl --from ./python

# Or run directly without installing
cd python
uv run wrds-dl --help
```

### With pip

```sh
pip install ./python
wrds-dl --help
```

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
...
```

For machine-readable output:

```sh
wrds-dl info --schema crsp --table dsf --json
```

## How it works

Connects directly to the WRDS PostgreSQL server using [psycopg](https://www.psycopg.org/) (v3) with bundled `libpq` for cross-platform portability (ARM macOS, x86 Linux, etc.).

**Downloads** use server-side cursors to stream rows without loading the entire result into memory:
- **Parquet**: rows are batched (10,000 per row group) and written with ZSTD compression via [PyArrow](https://arrow.apache.org/docs/python/).
- **CSV**: rows are streamed to disk via Python's `csv` module.

Progress is reported to stderr every 10,000 rows.

## Dependencies

| Package | Purpose |
|---|---|
| [`psycopg[binary]`](https://www.psycopg.org/) | PostgreSQL driver with bundled libpq |
| [`pyarrow`](https://arrow.apache.org/docs/python/) | Parquet writing with ZSTD compression |
| [`click`](https://click.palletsprojects.com/) | CLI framework |

## Development

```sh
cd python
uv sync
uv run pytest
uv run ruff check src tests
```
