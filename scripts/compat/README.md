# Parquet reader compatibility

`run.sh` checks that what `wrds-dl download --format parquet` writes is read back
correctly by the readers we use: **pyarrow**, **DuckDB** and Julia's **Parquet2.jl**.

```
scripts/compat/run.sh [OUT_DIR]          # WRDS_DL_COMPAT_TYPES=1 also prints the type table
```

It builds wrds-dl, loads `fixture.sql` into a throwaway Postgres 17 (server time zone
`America/New_York`, so time zones are exercised), exports each `cases/*.sql`, reads every
file with each reader (`read_py.py`, `read_jl.jl`) and compares the result with Postgres'
own `COPY` output and column types (`compare.py`). Needs nix; Julia on `PATH` is
optional (its environment is pinned by `Project.toml`/`Manifest.toml` here).

Run it before a release, or after changing `go/internal/export`.

## What is checked

For every case, reader and column: the column name, the reader's type category
(int, float, decimal, date, …) and every value. The expected values follow the type
mapping in `go/README.md`; in particular `numeric(p,s)` with p > 16 and unconstrained
`numeric` are DOUBLE, `char(n)` is trimmed, `±infinity` dates/timestamps are DuckDB's
extreme values, and repeated column names get `_1`, `_2`, … suffixes.

Cases: every supported type with typical, zero/empty, extreme, negative, non-finite and
NULL values plus odd column names (`types`); repeated names from a self-join
(`dup_names`); `sum`/`avg`/`min`/`max` results (`aggregates`); 1000 columns (`wide`);
30 005 rows over several row groups with high-cardinality, non-ASCII and 100 KB strings
(`many_rows`); an empty result (`empty`).

## Known reader limitations (accepted by `compare.py`)

- Parquet2.jl has no time-zone-aware `DateTime`: `timestamptz` loads as `DateTime` in UTC.
- Parquet2.jl converts µs timestamps through `Float64` seconds into a millisecond
  `DateTime`, so timestamps are compared to within 1 ms.

The DECIMAL limits themselves come from Parquet2.jl: it reads DECIMAL into 16-digit
`Dec64` and does not sign-extend FIXED_LEN_BYTE_ARRAY decimals, so wrds-dl writes
DECIMAL only up to p = 16, stored as INT32/INT64.
