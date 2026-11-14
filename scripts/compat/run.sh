#!/usr/bin/env bash
# Parquet reader-compatibility harness (see README.md).
#
# usage: scripts/compat/run.sh [OUT_DIR]
#
# Builds wrds-dl, loads fixture.sql into a throwaway Postgres 17 (server time zone
# America/New_York), exports every cases/*.sql to parquet, reads the files back with
# pyarrow, DuckDB and Julia's Parquet2.jl, and compares against Postgres' own output.
# Needs nix; Julia (on PATH) is optional — without it Parquet2.jl is skipped.
set -euo pipefail

here=$(cd "$(dirname "$0")" && pwd)

if [[ -z "${WRDS_DL_COMPAT_SHELL:-}" ]]; then
  exec nix shell --impure --expr '
    with import <nixpkgs> {};
    buildEnv {
      name = "wrds-dl-compat";
      paths = [ go postgresql_17 openssl (python3.withPackages (p: [ p.pyarrow p.duckdb ])) ];
    }' -c env WRDS_DL_COMPAT_SHELL=1 bash "$0" "$@"
fi

repo=$(cd "$here/../.." && pwd)
out=${1:-$(mktemp -d -t wrds-dl-compat.XXXXXX)}
out=$(mkdir -p "$out" && cd "$out" && pwd)
rm -rf "$out"/{pg,postgres,parquet,pyarrow,duckdb,parquet2jl}
mkdir -p "$out"/{postgres,parquet}
echo "output: $out"

echo "building wrds-dl"
(cd "$repo/go" && CGO_ENABLED=0 go build -o "$out/wrds-dl" .)

port=${WRDS_DL_COMPAT_PORT:-54329}
initdb -D "$out/pg" -U compat -A trust >/dev/null
openssl req -new -x509 -days 1 -nodes -subj /CN=localhost \
  -keyout "$out/pg/server.key" -out "$out/pg/server.crt" 2>/dev/null
chmod 600 "$out/pg/server.key"
pg_ctl -D "$out/pg" -l "$out/pg/log" -w start -o \
  "-p $port -c unix_socket_directories='' -c listen_addresses=localhost -c ssl=on -c TimeZone=America/New_York" >/dev/null
trap 'pg_ctl -D "$out/pg" -m fast stop >/dev/null' EXIT

export PGHOST=localhost PGPORT=$port PGUSER=compat PGDATABASE=compat PGPASSWORD=
createdb compat
psql -Xq -v ON_ERROR_STOP=1 -f "$here/fixture.sql"

for f in "$here"/cases/*.sql; do
  case=$(basename "$f" .sql)
  q=$(sed -e 's/;[[:space:]]*$//' "$f")
  printf '%s \\gdesc\n' "$q" | psql -XAt -F $'\t' -v ON_ERROR_STOP=1 > "$out/postgres/$case.desc"
  psql -XAt -v ON_ERROR_STOP=1 -c "COPY ($q) TO STDOUT" > "$out/postgres/$case.copy"
  "$out/wrds-dl" download --query "$q" --out "$out/parquet/$case.parquet" > "$out/parquet/$case.log" 2>&1 \
    || { echo "wrds-dl failed on $case:"; cat "$out/parquet/$case.log"; exit 1; }
done

echo "reading with pyarrow and DuckDB"
python3 -I "$here/read_py.py" "$out" "$out"/parquet/*.parquet

if command -v julia >/dev/null; then
  echo "reading with Parquet2.jl"
  julia --project="$here" -e 'using Pkg; Pkg.instantiate()' >/dev/null 2>&1
  julia --project="$here" "$here/read_jl.jl" "$out" "$out"/parquet/*.parquet
else
  echo "julia not on PATH: skipping Parquet2.jl"
fi

python3 -I "$here/compare.py" "$out" ${WRDS_DL_COMPAT_TYPES:+--types}
