# Changelog

Changes to the `wrds-dl` Go binary. Versions before 0.3.4 are listed only as git tags.

## 0.5.0 — 2026-11-14

### ⚠️ Potentially breaking: Parquet `numeric` is DOUBLE by default

Every Postgres `numeric` column is now written to Parquet as DOUBLE (#25). In 0.4.0,
`numeric(p,s)` with p ≤ 16 was written as DECIMAL. On WRDS that covers some of the most-used
columns, which change type with this release:

- CRSP `dsf`/`msf` `ret`, `retx`, `prc`, `bid`, `ask`, `vol`, …; `dsf_v2`/`msf_v2` `dlyret`,
  `dlyprc`, `mthret`, `mthprc`, …;
- Fama-French factors (`ff.factors_daily` `mktrf`, `smb`, `hml`, `rf`, `umd`, …).

Code that reads these columns as `Dec64` (Julia Parquet2.jl), `decimal.Decimal` (pandas) or
`DECIMAL(p,s)` (DuckDB, polars) now gets `Float64` / `float64` / `DOUBLE`. Values are the same
up to 15 significant digits, which covers the WRDS data checked: all of `comp.funda` and a
2023-Q1 slice of `comp.secd`.

To get the 0.4.0 output back, pass the new `--decimal` flag:

```sh
wrds-dl download --schema crsp --table msf --columns "permno,date,ret" --decimal --out msf.parquet
```

Why: WRDS declares `numeric` widths inconsistently, so under 0.4.0 one table could mix
DECIMAL and DOUBLE (`crsp.dsf` `prc` DECIMAL next to `shrout` DOUBLE), and DECIMAL is slow or
awkward in Julia and pandas.

CSV output is unchanged. In the TUI, the download dialog's "Exact decimals" field is the
equivalent of `--decimal` (default `n`).

### TUI: from browsing to a CLI command

- In the Preview pane, `j`/`k` move a column cursor and `space` marks columns; the download
  dialog's SELECT field starts with the marked columns.
- The download dialog shows the equivalent `wrds-dl download …` command as you type;
  `ctrl+y` copies it to the clipboard (OSC 52), and copied commands are printed to stdout
  when you quit.

### Other

- `scripts/compat/run.sh` checks both modes with pyarrow, DuckDB and Parquet2.jl.

## 0.4.0 — 2026-11-14

Parquet output changed (CSV unchanged):

- repeated column names get `_1`, `_2`, … suffixes (pyarrow could not read such files);
- `char(n)` values are written without their padding;
- `scripts/compat/run.sh`: reader-compatibility harness (pyarrow, DuckDB, Parquet2.jl).

## 0.3.4 — 2026-11-14

Parquet export rewritten on Apache Arrow (arrow-go):

- `numeric(p,s)` with p ≤ 16 is DECIMAL; unconstrained or wider `numeric` is DOUBLE (was
  VARCHAR, #19);
- dates and timestamps far from 1970 and `±infinity` are written correctly;
- `timestamp` (without time zone) is no longer marked UTC-adjusted;
- `time` is TIME; `json`, `uuid`, arrays and `interval` are their Postgres text.
