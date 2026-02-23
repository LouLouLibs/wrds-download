# Claude Code Skill: WRDS Download (Python)

A [Claude Code](https://claude.com/claude-code) skill for downloading WRDS data using natural language. This variant uses the **Python** `wrds-dl` CLI (via `uv`) — no pre-built binary required.

## Installation

Copy the skill folder into your Claude Code skills directory:

```sh
# Personal (all projects)
cp -r claude-skill-wrds-download-py ~/.claude/skills/wrds-download

# Or project-local
cp -r claude-skill-wrds-download-py .claude/skills/wrds-download
```

## Prerequisites

1. **`uv`** — install from https://docs.astral.sh/uv/
2. **`wrds-dl`** — install the Python CLI:
   ```sh
   uv tool install wrds-dl --from /path/to/wrds-download/python
   ```
3. **WRDS credentials** — set environment variables or save credentials:
   ```sh
   export PGUSER=your_username
   export PGPASSWORD=your_password
   ```

## Usage

```
/wrds-download CRSP daily stock data for 2020
```

Claude will inspect the table, show you the structure, do a dry run for large tables, and download to Parquet.
