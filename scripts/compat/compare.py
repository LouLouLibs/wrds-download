"""Compare what each parquet reader saw against Postgres' own output.

usage: python3 -I compare.py OUT_DIR [--decimal] [--types]

OUT_DIR/postgres/<case>.desc   column names and types (psql \\gdesc, tab-separated)
OUT_DIR/postgres/<case>.copy   rows (COPY ... TO STDOUT, text format)
OUT_DIR/<reader>/<case>.json   written by read_py.py / read_jl.jl:
    {"rows": n, "columns": [{"name", "type", "kind", "values"}]} or {"error": msg}

Readers encode values as: int/bool/string as JSON; float as a string parsed by
float(); decimal as a string; date as days since 1970-01-01; timestamp and time as
microseconds (since the epoch / midnight).

The expected value of each column is derived from the Postgres type and text output,
following what wrds-dl promises (go/README.md, "Parquet"); --decimal checks files
written with `wrds-dl download --decimal`. Known reader limitations
are listed in READER_LIMITS. Exit status is 1 if anything differs.
"""

import datetime as dt
import json
import math
import os
import re
import struct
import sys
from decimal import Decimal

DATE_INF = 2**31 - 1  # wrds-dl writes ±infinity as DuckDB's extreme values
TS_INF = 2**63 - 1
EPOCH = dt.datetime(1970, 1, 1)
EPOCH_DATE = dt.date(1970, 1, 1)
MAX_DECIMAL_PRECISION = 16  # parquet.go maxDecimalPrecision

READERS = ("pyarrow", "duckdb", "parquet2jl")

# Known reader limitations, accepted by kind_ok/same below and printed with the result.
READER_LIMITS = {
    "parquet2jl": [
        "Julia has no time-zone-aware DateTime: timestamptz loads as DateTime (UTC).",
        "Parquet2.jl converts µs timestamps through Float64 seconds into a millisecond "
        "DateTime: values are compared to within 1 ms.",
    ],
}


def unescape_copy(field: str):
    """Decode one field of COPY text format (NULL is \\N)."""
    if field == r"\N":
        return None
    out, i = [], 0
    simple = {"b": "\b", "f": "\f", "n": "\n", "r": "\r", "t": "\t", "v": "\v", "\\": "\\"}
    while i < len(field):
        c = field[i]
        if c == "\\" and i + 1 < len(field):
            nxt = field[i + 1]
            if nxt in simple:
                out.append(simple[nxt])
                i += 2
                continue
            m = re.match(r"[0-7]{1,3}", field[i + 1:])
            if m:
                out.append(chr(int(m.group(), 8)))
                i += 1 + len(m.group())
                continue
            out.append(nxt)
            i += 2
            continue
        out.append(c)
        i += 1
    return "".join(out)


def unique_names(names):
    """Mirror of parquet.go uniqueNames."""
    taken, seen, out = set(names), set(), []
    for n in names:
        name = n
        if name in seen:
            k = 1
            while f"{n}_{k}" in taken:
                k += 1
            name = f"{n}_{k}"
            taken.add(name)
        seen.add(name)
        out.append(name)
    return out


def numeric_typmod(pg_type: str):
    m = re.fullmatch(r"numeric\((\d+),(-?\d+)\)", pg_type)
    return (int(m.group(1)), int(m.group(2))) if m else None


def expected_kind(pg_type: str, decimal: bool) -> str:
    if pg_type == "boolean":
        return "bool"
    if pg_type in ("smallint", "integer", "bigint"):
        return "int"
    if pg_type == "real":
        return "float32"
    if pg_type == "double precision":
        return "float"
    if pg_type.startswith("numeric"):
        tm = numeric_typmod(pg_type)
        if decimal and tm and tm[0] <= MAX_DECIMAL_PRECISION and 0 <= tm[1] <= tm[0]:
            return "decimal"
        return "float"
    if pg_type == "date":
        return "date"
    if pg_type == "timestamp without time zone":
        return "timestamp"
    if pg_type == "timestamp with time zone":
        return "timestamptz"
    if pg_type.startswith("time without time zone"):
        return "time"
    return "string"


def micros(d: dt.datetime) -> int:
    return (d - EPOCH) // dt.timedelta(microseconds=1)


def expected_value(pg_type: str, kind: str, text):
    """Expected canonical value from Postgres' text output."""
    if text is None:
        return None
    if kind == "bool":
        return text == "t"
    if kind == "int":
        return int(text)
    if kind in ("float", "float32"):
        return float(text)
    if kind == "decimal":
        return None if text == "NaN" else Decimal(text)  # DECIMAL cannot hold NaN
    if kind == "date":
        if text in ("infinity", "-infinity"):
            return DATE_INF if text == "infinity" else -DATE_INF
        return (dt.date.fromisoformat(text) - EPOCH_DATE).days
    if kind in ("timestamp", "timestamptz"):
        if text in ("infinity", "-infinity"):
            return TS_INF if text == "infinity" else -TS_INF
        d = dt.datetime.fromisoformat(text)
        if d.tzinfo is not None:
            d = d.astimezone(dt.timezone.utc).replace(tzinfo=None)
        return micros(d)
    if kind == "time":
        h, m, s = text.split(":")
        return round((int(h) * 3600 + int(m) * 60) * 1_000_000 + Decimal(s) * 1_000_000)
    if pg_type.startswith("character(") or pg_type == "character":
        return text.rstrip(" ")  # char(n) padding is trimmed
    return text


def float_bits(x: float, kind: str):
    if math.isnan(x):
        return "nan"
    return struct.pack("<f" if kind == "float32" else "<d", x)


def same(reader: str, kind: str, want, got) -> bool:
    if want is None or got is None:
        return want is None and got is None
    if kind in ("float", "float32"):
        return float_bits(want, kind) == float_bits(float(got), kind)
    if kind == "decimal":
        return want == Decimal(str(got))
    if reader == "parquet2jl" and kind in ("timestamp", "timestamptz"):
        if abs(want) == TS_INF:  # far beyond Float64 µs precision; must stay extreme
            return got * want > 0 and abs(got) > 2**62
        return abs(want - got) <= 1000
    return want == got


def kind_ok(reader: str, want: str, got: str) -> bool:
    if reader == "parquet2jl" and want == "timestamptz":
        return got == "timestamp"
    return want == got


def load_postgres(out: str, case: str):
    with open(os.path.join(out, "postgres", case + ".desc"), encoding="utf-8") as f:
        desc = [line.rstrip("\n").split("\t") for line in f if line.strip()]
    rows = []
    with open(os.path.join(out, "postgres", case + ".copy"), encoding="utf-8") as f:
        for line in f:
            rows.append([unescape_copy(x) for x in line.rstrip("\n").split("\t")])
    return desc, rows


def compare_case(out: str, case: str, reader: str, decimal: bool, problems: list):
    desc, rows = load_postgres(out, case)
    path = os.path.join(out, reader, case + ".json")
    if not os.path.exists(path):
        problems.append(f"{case} [{reader}]: no output")
        return None
    with open(path, encoding="utf-8") as f:
        res = json.load(f)
    if "error" in res:
        problems.append(f"{case} [{reader}]: read failed: {res['error'][:300]}")
        return None
    cols = res["columns"]
    if len(cols) != len(desc):
        problems.append(f"{case} [{reader}]: {len(cols)} columns, want {len(desc)}")
        return res
    if res["rows"] != len(rows):
        problems.append(f"{case} [{reader}]: {res['rows']} rows, want {len(rows)}")
        return res
    names = unique_names([d[0] for d in desc])
    for ci, ((_, pg_type), col, name) in enumerate(zip(desc, cols, names)):
        label = f"{case}.{name} [{reader}]"
        if col["name"] != name:
            problems.append(f"{label}: column named {col['name']!r}, want {name!r}")
        kind = expected_kind(pg_type, decimal)
        if not kind_ok(reader, kind, col["kind"]):
            problems.append(f"{label}: {pg_type} read as {col['type']} ({col['kind']}), want {kind}")
            continue
        bad = []
        for ri, row in enumerate(rows):
            want = expected_value(pg_type, kind, row[ci])
            got = col["values"][ri]
            if not same(reader, kind, want, got):
                bad.append(f"row {ri}: got {got!r}, want {want!r} (postgres: {row[ci]!r})")
        if bad:
            more = f" (+{len(bad) - 3} more)" if len(bad) > 3 else ""
            problems.append(f"{label} {pg_type}:\n      " + "\n      ".join(bad[:3]) + more)
    return res


def main():
    out = sys.argv[1]
    show_types = "--types" in sys.argv
    decimal = "--decimal" in sys.argv
    readers = [r for r in READERS if os.path.isdir(os.path.join(out, r))]
    cases = sorted(f[:-5] for f in os.listdir(os.path.join(out, "postgres")) if f.endswith(".desc"))
    problems, types = [], {}
    for case in cases:
        for reader in readers:
            res = compare_case(out, case, reader, decimal, problems)
            if case == "types" and res and "columns" in res:
                types[reader] = [c["type"] for c in res["columns"]]

    if show_types and types:
        desc, _ = load_postgres(out, "types")
        print("| postgres | " + " | ".join(readers) + " |")
        print("|---" * (len(readers) + 1) + "|")
        for i, (name, pg_type) in enumerate(desc):
            print(f"| `{name}` {pg_type} | " + " | ".join(
                f"`{types[r][i]}`" if r in types and i < len(types[r]) else "?" for r in readers) + " |")
        print()

    for reader, notes in READER_LIMITS.items():
        if reader in readers:
            for n in notes:
                print(f"note [{reader}]: {n}")
    print(f"\n{len(cases)} cases × {len(readers)} readers ({', '.join(readers)})")
    if problems:
        print(f"{len(problems)} problem(s):")
        for p in problems:
            print("  - " + p)
        sys.exit(1)
    print("all readers match Postgres")


if __name__ == "__main__":
    main()
