---
name: wrds-download
description: Download data from the WRDS (Wharton Research Data Services) PostgreSQL database to local Parquet or CSV files. Use when the user asks to get data from WRDS, download financial data, or mentions WRDS schemas like crsp, comp, optionm, ibes, etc.
allowed-tools: Bash(wrds-dl *), Bash(uv run *wrds*), Read, Grep
argument-hint: [description of data needed]
---

# WRDS Data Download (Python)

You help users download data from the Wharton Research Data Services (WRDS) PostgreSQL database using the `wrds-dl` Python CLI tool (via `uv`).

## Prerequisites

The user must have:
- **`uv`** installed (https://docs.astral.sh/uv/)
- **WRDS credentials** configured via one of:
  - Environment variables: `PGUSER` and `PGPASSWORD`
  - Saved credentials at `~/.config/wrds-dl/credentials`
  - Standard `~/.pgpass` file

Run the CLI with `uv run wrds-dl` from the `python/` directory of the wrds-download repo, or `wrds-dl` if installed via `uv tool install`.

If `wrds-dl` is not found and `uv` is available, install it:
```bash
uv tool install wrds-dl --from /path/to/wrds-download/python
```

## Workflow

Follow these steps for every download request:

### Step 1: Identify the table

Parse the user's request to determine the WRDS schema and table. Common mappings:

| Dataset | Schema | Key Tables |
|---------|--------|------------|
| CRSP daily stock | `crsp` | `dsf` (daily), `msf` (monthly), `dsi` (index) |
| CRSP events | `crsp` | `dsedelist`, `stocknames` |
| Compustat annual | `comp` | `funda` |
| Compustat quarterly | `comp` | `fundq` |
| Compustat global | `comp_global_daily` | `g_funda`, `g_fundq` |
| IBES | `ibes` | `statsum_epsus`, `actu_epsus` |
| OptionMetrics | `optionm` | `opprcd` (prices), `secprd` (security) |
| TAQ | `taqmsec` | `ctm_YYYYMMDD` |
| CRSP/Compustat merged | `crsp` | `ccmxpf_linktable` |
| BoardEx | `boardex` | `na_wrds_company_profile` |
| Institutional (13F) | `tfn` | `s34` |
| Audit Analytics | `audit` | `auditnonreli` |
| Ravenpack | `ravenpack` | `rpa_djnw` |
| Bank Regulatory | `bank` | `call_schedule_rc`, `bhck` |

If unsure which table, ask the user or use `wrds-dl info` to explore.

### Step 2: Inspect the table

Always run `wrds-dl info` first to understand the table structure:

```bash
wrds-dl info --schema <schema> --table <table>
```

Use the output to:
- Confirm the table exists and has the expected columns
- Note column names for the user's requested variables
- Check the estimated row count to warn about large downloads

For JSON output (useful for parsing): `wrds-dl info --schema <schema> --table <table> --json`

### Step 3: Dry run

For tables with more than 1 million estimated rows, or when a WHERE clause is involved, always do a dry run first:

```bash
wrds-dl download --schema <schema> --table <table> \
  --columns "<cols>" --where "<filter>" --dry-run
```

Show the user the row count and sample rows. Ask for confirmation before proceeding if the row count is very large (>10M rows).

### Step 4: Download

Build and run the download command:

```bash
wrds-dl download \
  --schema <schema> \
  --table <table> \
  --columns "<comma-separated columns>" \
  --where "<SQL filter>" \
  --out <output_file> \
  --format <parquet|csv>
```

#### Defaults and conventions
- **Format**: Use Parquet unless the user asks for CSV. Parquet is smaller and faster.
- **Output path**: Name the file descriptively, e.g., `crsp_dsf_2020.parquet` or `comp_funda_2010_2023.parquet`.
- **Columns**: Select only the columns the user needs. Don't use `*` on wide tables — ask what variables they need.
- **Limit**: Use `--limit` for testing. Suggest `--limit 1000` if the user is exploring.

#### Common filters
- Date ranges: `--where "date >= '2020-01-01' AND date < '2021-01-01'"`
- Specific firms by permno: `--where "permno IN (10107, 93436)"`
- Specific firms by gvkey: `--where "gvkey IN ('001690', '012141')"`
- Fiscal year: `--where "fyear >= 2010 AND fyear <= 2023"`

### Step 5: Verify

After download completes, confirm the file was created and report its size:

```bash
ls -lh <output_file>
```

## Error handling

- **Authentication errors**: Remind the user to set `PGUSER`/`PGPASSWORD` or configure `~/.config/wrds-dl/credentials`.
- **Table not found**: Use `wrds-dl info` to check schema/table names. WRDS schemas and table names are lowercase.
- **Timeout on large tables**: Suggest adding a `--where` filter or `--limit` to reduce the result set.
- **Duo 2FA prompt**: The connection triggers a Duo push. Tell the user to approve it on their phone.

## Example interactions

**User**: "Download CRSP daily stock data for 2020"
-> `wrds-dl info --schema crsp --table dsf`
-> `wrds-dl download --schema crsp --table dsf --where "date >= '2020-01-01' AND date < '2021-01-01'" --out crsp_dsf_2020.parquet`

**User**: "Get Compustat annual fundamentals, just gvkey, datadate, and sales"
-> `wrds-dl info --schema comp --table funda`
-> `wrds-dl download --schema comp --table funda --columns "gvkey,datadate,sale" --out comp_funda.parquet`

**User**: "I need IBES analyst estimates"
-> `wrds-dl info --schema ibes --table statsum_epsus`
-> Ask what date range and variables they need, then download.
