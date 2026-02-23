"""Export query results to Parquet or CSV with streaming and progress."""

from __future__ import annotations

import csv
from decimal import Decimal
from typing import Callable

import psycopg
import pyarrow as pa
import pyarrow.parquet as pq

from wrds_dl.db import dsn_from_env

ROW_GROUP_SIZE = 10_000

# Map PostgreSQL type OIDs to PyArrow types.
_PG_OID_TO_ARROW: dict[int, pa.DataType] = {
    16: pa.bool_(),        # bool
    21: pa.int32(),        # int2
    23: pa.int32(),        # int4
    20: pa.int64(),        # int8
    700: pa.float32(),     # float4
    701: pa.float64(),     # float8
    1082: pa.date32(),     # date
    1114: pa.timestamp("us"),  # timestamp
    1184: pa.timestamp("us", tz="UTC"),  # timestamptz
}


def _arrow_type_for_oid(oid: int) -> pa.DataType:
    return _PG_OID_TO_ARROW.get(oid, pa.string())


def export_data(
    query: str,
    out_path: str,
    fmt: str = "parquet",
    progress_fn: Callable[[int], None] | None = None,
) -> None:
    """Run *query* against WRDS and write results to *out_path*."""
    conn = psycopg.connect(dsn_from_env())
    try:
        with conn.cursor(name="wrds_export") as cur:
            cur.itersize = ROW_GROUP_SIZE
            cur.execute(query)

            if cur.description is None:
                raise RuntimeError("Query returned no columns")

            col_names = [desc.name for desc in cur.description]
            col_oids = [desc.type_code for desc in cur.description]

            if fmt == "csv":
                _write_csv(cur, col_names, out_path, progress_fn)
            else:
                _write_parquet(cur, col_names, col_oids, out_path, progress_fn)
    finally:
        conn.close()


def _write_csv(
    cur: psycopg.Cursor,
    col_names: list[str],
    out_path: str,
    progress_fn: Callable[[int], None] | None,
) -> None:
    with open(out_path, "w", newline="") as f:
        writer = csv.writer(f)
        writer.writerow(col_names)
        total = 0
        for row in cur:
            writer.writerow(_format_row(row))
            total += 1
            if progress_fn and total % ROW_GROUP_SIZE == 0:
                progress_fn(total)


def _write_parquet(
    cur: psycopg.Cursor,
    col_names: list[str],
    col_oids: list[int],
    out_path: str,
    progress_fn: Callable[[int], None] | None,
) -> None:
    arrow_types = [_arrow_type_for_oid(oid) for oid in col_oids]
    schema = pa.schema([(name, typ) for name, typ in zip(col_names, arrow_types)])

    writer = pq.ParquetWriter(out_path, schema, compression="zstd")
    try:
        batch_rows: list[tuple] = []
        total = 0

        for row in cur:
            batch_rows.append(row)
            if len(batch_rows) >= ROW_GROUP_SIZE:
                _flush_batch(writer, schema, batch_rows, col_names)
                total += len(batch_rows)
                batch_rows = []
                if progress_fn:
                    progress_fn(total)

        if batch_rows:
            _flush_batch(writer, schema, batch_rows, col_names)
            total += len(batch_rows)
    finally:
        writer.close()


def _flush_batch(
    writer: pq.ParquetWriter,
    schema: pa.Schema,
    rows: list[tuple],
    col_names: list[str],
) -> None:
    """Convert a batch of rows into a PyArrow table and write it."""
    columns: dict[str, list] = {name: [] for name in col_names}
    for row in rows:
        for i, val in enumerate(row):
            # Strip trailing zeros from Decimal values (numeric columns)
            # so output matches Go's pgx behaviour.
            if isinstance(val, Decimal):
                val = str(val.normalize())
            columns[col_names[i]].append(val)

    arrays = []
    for i, name in enumerate(col_names):
        try:
            arrays.append(pa.array(columns[name], type=schema.field(name).type))
        except (pa.ArrowInvalid, pa.ArrowTypeError):
            # Fallback: convert to strings
            arrays.append(pa.array([str(v) if v is not None else None for v in columns[name]],
                                   type=pa.string()))

    table = pa.table(dict(zip(col_names, arrays)))
    writer.write_table(table)


def _format_row(row: tuple) -> list[str]:
    """Format a row for CSV output."""
    out = []
    for v in row:
        if v is None:
            out.append("")
        elif isinstance(v, Decimal):
            out.append(str(v.normalize()))
        else:
            out.append(str(v))
    return out
