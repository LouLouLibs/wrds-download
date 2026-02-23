"""PostgreSQL connection, DSN construction, and metadata queries."""

from __future__ import annotations

import os
from dataclasses import dataclass, field

import psycopg


def _getenv(key: str, fallback: str = "") -> str:
    return os.environ.get(key) or fallback


def dsn_from_env() -> str:
    """Build a PostgreSQL connection string from standard PG* environment variables."""
    host = _getenv("PGHOST", "wrds-pgdata.wharton.upenn.edu")
    port = _getenv("PGPORT", "9737")
    user = _getenv("PGUSER")
    password = _getenv("PGPASSWORD")
    database = _getenv("PGDATABASE", "wrds")

    if not user:
        raise RuntimeError("PGUSER not set")

    dsn = f"host={host} port={port} user={user} sslmode=require"
    if password:
        dsn += f" password={password}"
    if database:
        dsn += f" dbname={database}"
    return dsn


def connect() -> psycopg.Connection:
    """Return a psycopg connection using DSN from environment variables."""
    return psycopg.connect(dsn_from_env())


def quote_ident(s: str) -> str:
    """Quote a PostgreSQL identifier to prevent SQL injection."""
    return '"' + s.replace('"', '""') + '"'


def build_query(
    schema: str,
    table: str,
    columns: str = "*",
    where: str = "",
    limit: int = 0,
) -> str:
    """Build a SELECT query with quoted identifiers."""
    if columns and columns != "*":
        parts = [quote_ident(c.strip()) for c in columns.split(",")]
        sel = ", ".join(parts)
    else:
        sel = "*"

    q = f"SELECT {sel} FROM {quote_ident(schema)}.{quote_ident(table)}"
    if where:
        q += f" WHERE {where}"
    if limit > 0:
        q += f" LIMIT {limit}"
    return q


@dataclass
class ColumnMeta:
    name: str
    data_type: str
    nullable: bool
    description: str


@dataclass
class TableMeta:
    schema: str
    table: str
    comment: str = ""
    row_count: int = 0
    size: str = ""
    columns: list[ColumnMeta] = field(default_factory=list)


def table_meta(conn: psycopg.Connection, schema: str, table: str) -> TableMeta:
    """Fetch catalog metadata for a table (no data scan)."""
    meta = TableMeta(schema=schema, table=table)

    # Table-level stats (best effort).
    with conn.cursor() as cur:
        cur.execute(
            """
            SELECT c.reltuples::bigint,
                   COALESCE(pg_size_pretty(pg_total_relation_size(c.oid)), ''),
                   COALESCE(obj_description(c.oid), '')
            FROM pg_class c
            JOIN pg_namespace n ON n.oid = c.relnamespace
            WHERE n.nspname = %s AND c.relname = %s
            """,
            (schema, table),
        )
        row = cur.fetchone()
        if row:
            meta.row_count, meta.size, meta.comment = row

    # Column metadata with descriptions from pg_description.
    with conn.cursor() as cur:
        cur.execute(
            """
            SELECT a.attname,
                   pg_catalog.format_type(a.atttypid, a.atttypmod),
                   NOT a.attnotnull,
                   COALESCE(d.description, '')
            FROM pg_attribute a
            JOIN pg_class c ON a.attrelid = c.oid
            JOIN pg_namespace n ON c.relnamespace = n.oid
            LEFT JOIN pg_description d ON d.objoid = c.oid AND d.objsubid = a.attnum
            WHERE n.nspname = %s AND c.relname = %s
              AND a.attnum > 0 AND NOT a.attisdropped
            ORDER BY a.attnum
            """,
            (schema, table),
        )
        for name, dtype, nullable, desc in cur:
            meta.columns.append(ColumnMeta(name, dtype, nullable, desc))

    return meta
