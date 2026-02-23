"""Integration test: download a small CRSP MSF sample and verify output.

Requires WRDS credentials (PGUSER/PGPASSWORD or ~/.config/wrds-dl/credentials).
Skipped automatically when credentials are unavailable.

If the Go wrds-dl binary is found, downloads the same data with both
implementations and asserts their content hashes match.
"""

from __future__ import annotations

import hashlib
import os
import subprocess
import tempfile
from pathlib import Path

import pyarrow.parquet as pq
import pytest

from wrds_dl.config import load_credentials

# A narrow, deterministic query: 10 rows from crsp.msf for Jan 2020.
QUERY = (
    "SELECT permno, date, prc, ret, shrout "
    "FROM crsp.msf "
    "WHERE date = '2020-01-31' "
    "ORDER BY permno "
    "LIMIT 10"
)

REPO_ROOT = Path(__file__).resolve().parents[2]
GO_BINARY = REPO_ROOT / "wrds-dl"  # pre-built binary at repo root


def _has_credentials() -> bool:
    if os.environ.get("PGUSER"):
        return True
    user, pw, _ = load_credentials()
    return bool(user and pw)


pytestmark = pytest.mark.skipif(
    not _has_credentials(),
    reason="WRDS credentials not available",
)


def _content_hash(parquet_path: str) -> str:
    """Read a parquet file, sort deterministically, and return a SHA-256 of the content.

    Converts all values to their repr() for a canonical representation
    that is independent of the parquet writer (parquet-go vs pyarrow).
    """
    table = pq.read_table(parquet_path)
    # Normalize column order alphabetically.
    col_names = sorted(table.column_names)
    table = table.select(col_names)
    # Sort rows by all columns.
    sort_keys = [(col, "ascending") for col in col_names]
    table = table.sort_by(sort_keys)
    # Hash a canonical string representation of every cell.
    h = hashlib.sha256()
    h.update(",".join(col_names).encode())
    for i in range(table.num_rows):
        for col_name in col_names:
            val = table.column(col_name)[i].as_py()
            h.update(repr(val).encode())
            h.update(b"|")
        h.update(b"\n")
    return h.hexdigest()


def test_python_download_parquet():
    """Download a small sample with the Python CLI and verify the parquet output."""
    with tempfile.TemporaryDirectory() as tmpdir:
        out = os.path.join(tmpdir, "test_py.parquet")

        from click.testing import CliRunner
        from wrds_dl.cli import cli

        runner = CliRunner()
        result = runner.invoke(cli, ["download", "--query", QUERY, "--out", out])
        assert result.exit_code == 0, f"Python download failed: {result.output}"

        # Verify parquet file.
        table = pq.read_table(out)
        assert table.num_rows == 10
        assert set(table.column_names) == {"permno", "date", "prc", "ret", "shrout"}

        py_hash = _content_hash(out)
        assert len(py_hash) == 64  # valid sha256


@pytest.mark.skipif(
    not GO_BINARY.is_file(),
    reason=f"Go binary not found at {GO_BINARY}",
)
def test_go_python_parity():
    """Download the same data with Go and Python, assert content hashes match."""
    with tempfile.TemporaryDirectory() as tmpdir:
        py_out = os.path.join(tmpdir, "py.parquet")
        go_out = os.path.join(tmpdir, "go.parquet")

        # Python download.
        from click.testing import CliRunner
        from wrds_dl.cli import cli

        runner = CliRunner()
        result = runner.invoke(cli, ["download", "--query", QUERY, "--out", py_out])
        assert result.exit_code == 0, f"Python download failed: {result.output}"

        # Go download.
        env = os.environ.copy()
        proc = subprocess.run(
            [str(GO_BINARY), "download", "--query", QUERY, "--out", go_out],
            capture_output=True,
            text=True,
            env=env,
            timeout=60,
        )
        assert proc.returncode == 0, f"Go download failed: {proc.stderr}"

        # Compare content hashes.
        py_hash = _content_hash(py_out)
        go_hash = _content_hash(go_out)

        # Read both tables for diagnostics on failure.
        py_table = pq.read_table(py_out)
        go_table = pq.read_table(go_out)

        assert py_table.num_rows == go_table.num_rows, (
            f"Row count mismatch: Python={py_table.num_rows}, Go={go_table.num_rows}"
        )
        assert set(py_table.column_names) == set(go_table.column_names), (
            f"Column mismatch: Python={py_table.column_names}, Go={go_table.column_names}"
        )
        assert py_hash == go_hash, (
            f"Content hash mismatch:\n"
            f"  Python: {py_hash}\n"
            f"  Go:     {go_hash}\n"
            f"  Python schema:\n{py_table.schema}\n"
            f"  Go schema:\n{go_table.schema}\n"
        )
