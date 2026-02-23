"""Click CLI — download and info subcommands matching the Go wrds-dl interface."""

from __future__ import annotations

import json
import sys

import click
import psycopg

from wrds_dl.config import apply_credentials
from wrds_dl.db import build_query, connect, dsn_from_env, quote_ident, table_meta


@click.group()
def cli() -> None:
    """Download data from the WRDS PostgreSQL database to Parquet or CSV."""


@cli.command()
@click.option("--schema", default="", help="Schema name (e.g. crsp)")
@click.option("--table", default="", help="Table name (e.g. dsf)")
@click.option("-c", "--columns", default="*", help="Columns to select (comma-separated, default *)")
@click.option("--where", "where_clause", default="", help="SQL WHERE clause (without the WHERE keyword)")
@click.option("--query", default="", help="Full SQL query (overrides --schema/--table/--where)")
@click.option("--out", default="", help="Output file path (required unless --dry-run)")
@click.option("--format", "fmt", default="", help="Output format: parquet or csv (inferred from extension)")
@click.option("--limit", default=0, type=int, help="Limit number of rows (0 = no limit)")
@click.option("--dry-run", is_flag=True, help="Preview query, row count, and first 5 rows")
def download(
    schema: str,
    table: str,
    columns: str,
    where_clause: str,
    query: str,
    out: str,
    fmt: str,
    limit: int,
    dry_run: bool,
) -> None:
    """Download WRDS data to Parquet or CSV."""
    apply_credentials()

    # Build query.
    if query:
        sql = query
    elif schema and table:
        sql = build_query(schema, table, columns, where_clause, limit)
    else:
        raise click.UsageError("Either --query or both --schema and --table must be specified")

    if dry_run:
        _run_dry_run(sql)
        return

    if not out:
        raise click.UsageError('Required option "--out" not provided')

    # Resolve format.
    resolved_fmt = fmt.lower() if fmt else ("csv" if out.lower().endswith(".csv") else "parquet")

    click.echo(f"Exporting to {out} ({resolved_fmt})...", err=True)

    from wrds_dl.export import export_data

    def progress(rows: int) -> None:
        click.echo(f"Exported {rows} rows...", err=True)

    export_data(sql, out, resolved_fmt, progress)
    click.echo(f"Done: {out}", err=True)


def _run_dry_run(sql: str) -> None:
    """Print query, row count, and first 5 rows."""
    conn = psycopg.connect(dsn_from_env())
    try:
        with conn.cursor() as cur:
            click.echo("Query:")
            click.echo(f"  {sql}")
            click.echo()

            # Row count.
            cur.execute(f"SELECT count(*) FROM ({sql}) sub")
            row = cur.fetchone()
            count = row[0] if row else 0
            click.echo(f"Row count: {count}")
            click.echo()

            # Preview first 5 rows.
            cur.execute(f"SELECT * FROM ({sql}) sub LIMIT 5")
            if cur.description is None:
                return

            col_names = [desc.name for desc in cur.description]
            rows = cur.fetchall()

            # Calculate column widths.
            widths = [len(name) for name in col_names]
            str_rows = []
            for row in rows:
                cells = [str(v) if v is not None else "NULL" for v in row]
                str_rows.append(cells)
                for i, cell in enumerate(cells):
                    widths[i] = max(widths[i], len(cell))

            # Print header and rows.
            header = "  ".join(name.ljust(widths[i]) for i, name in enumerate(col_names))
            click.echo(header)
            for cells in str_rows:
                click.echo("  ".join(cell.ljust(widths[i]) for i, cell in enumerate(cells)))
    finally:
        conn.close()


@cli.command()
@click.option("--schema", required=True, help="Schema name (required)")
@click.option("--table", required=True, help="Table name (required)")
@click.option("--json", "as_json", is_flag=True, help="Output as JSON")
def info(schema: str, table: str, as_json: bool) -> None:
    """Show table metadata (columns, types, row count)."""
    apply_credentials()

    conn = connect()
    try:
        meta = table_meta(conn, schema, table)
    finally:
        conn.close()

    if as_json:
        _print_info_json(meta)
    else:
        _print_info_table(meta)


def _print_info_json(meta) -> None:
    data = {
        "schema": meta.schema,
        "table": meta.table,
        "comment": meta.comment or None,
        "row_count": meta.row_count,
        "size": meta.size or None,
        "columns": [
            {
                "name": c.name,
                "type": c.data_type,
                "nullable": c.nullable,
                **({"description": c.description} if c.description else {}),
            }
            for c in meta.columns
        ],
    }
    # Match Go: omit null keys
    data = {k: v for k, v in data.items() if v is not None}
    click.echo(json.dumps(data, indent=2))


def _print_info_table(meta) -> None:
    click.echo(f"{meta.schema}.{meta.table}")
    if meta.comment:
        click.echo(f"  {meta.comment}")

    parts = []
    if meta.row_count > 0:
        parts.append(f"~{meta.row_count} rows")
    if meta.size:
        parts.append(meta.size)
    if parts:
        click.echo(f"  {', '.join(parts)}")

    click.echo()

    # Column table with tab-aligned output.
    widths = [4, 4, 8, 11]  # NAME, TYPE, NULLABLE, DESCRIPTION minimums
    rows = []
    for c in meta.columns:
        nullable = "YES" if c.nullable else "NO"
        row = [c.name, c.data_type, nullable, c.description]
        rows.append(row)
        for i, cell in enumerate(row):
            widths[i] = max(widths[i], len(cell))

    header = "  ".join(
        label.ljust(widths[i])
        for i, label in enumerate(["NAME", "TYPE", "NULLABLE", "DESCRIPTION"])
    )
    click.echo(header)
    for row in rows:
        click.echo("  ".join(cell.ljust(widths[i]) for i, cell in enumerate(row)))
