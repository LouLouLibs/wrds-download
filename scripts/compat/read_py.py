"""Read parquet files with pyarrow and DuckDB and dump them in the harness' common form.

usage: python3 -I read_py.py OUT_DIR FILE.parquet...
Writes OUT_DIR/pyarrow/<case>.json and OUT_DIR/duckdb/<case>.json, each
{"columns": [{"name", "type", "kind", "values"}]}; see compare.py for the value encoding.
"""

import json
import os
import sys

import duckdb
import pyarrow as pa
import pyarrow.parquet as pq


def kind_of(t: pa.DataType) -> str:
    if pa.types.is_boolean(t):
        return "bool"
    if pa.types.is_integer(t):
        return "int"
    if pa.types.is_float32(t):
        return "float32"
    if pa.types.is_float64(t):
        return "float"
    if pa.types.is_decimal(t):
        return "decimal"
    if pa.types.is_date32(t):
        return "date"
    if pa.types.is_timestamp(t):
        return "timestamptz" if t.tz else "timestamp"
    if pa.types.is_time(t):
        return "time"
    if pa.types.is_string(t) or pa.types.is_large_string(t):
        return "string"
    return "other"


def values_of(col: pa.ChunkedArray, kind: str) -> list:
    """Encode values without going through Python date/datetime (which cannot hold
    every Parquet value): temporal kinds as integers in their unit."""
    t = col.type
    if kind == "date":
        col = col.cast(pa.int32())
    elif kind in ("timestamp", "timestamptz"):
        col = col.cast(pa.timestamp("us", tz=t.tz)).cast(pa.int64())
    elif kind == "time":
        col = col.cast(pa.time64("us")).cast(pa.int64())
    vals = col.to_pylist()
    if kind in ("float", "float32"):
        return [None if v is None else repr(v) for v in vals]
    if kind == "decimal":
        return [None if v is None else str(v) for v in vals]
    return vals


def dump(table: pa.Table, types: list) -> dict:
    cols = []
    for i, field in enumerate(table.schema):
        kind = kind_of(field.type)
        cols.append({"name": field.name, "type": types[i], "kind": kind,
                     "values": values_of(table.column(i), kind)})
    return {"rows": table.num_rows, "columns": cols}


def main() -> None:
    out, files = sys.argv[1], sys.argv[2:]
    for reader in ("pyarrow", "duckdb"):
        os.makedirs(os.path.join(out, reader), exist_ok=True)
    con = duckdb.connect()
    con.execute("SET TimeZone = 'UTC'")
    for path in files:
        case = os.path.splitext(os.path.basename(path))[0]
        results = {}
        try:
            t = pq.read_table(path)
            results["pyarrow"] = dump(t, [str(f.type) for f in t.schema])
        except Exception as e:  # report, don't stop the other readers
            results["pyarrow"] = {"error": f"{type(e).__name__}: {e}"}
        try:
            rel = con.read_parquet(path)
            results["duckdb"] = dump(rel.to_arrow_table(), [str(t) for t in rel.types])
        except Exception as e:
            results["duckdb"] = {"error": f"{type(e).__name__}: {e}"}
        for reader, res in results.items():
            with open(os.path.join(out, reader, case + ".json"), "w") as f:
                json.dump(res, f)


if __name__ == "__main__":
    main()
