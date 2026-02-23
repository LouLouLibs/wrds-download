# Claude Skill: wrds-download

A [Claude Code](https://claude.com/claude-code) skill that lets you download data from WRDS using natural language.

## Installation

### Option 1: Copy into your project

```bash
cp -r claude-skill-wrds-download .claude/skills/wrds-download
```

### Option 2: Copy to your personal skills (works across all projects)

```bash
cp -r claude-skill-wrds-download ~/.claude/skills/wrds-download
```

## Prerequisites

1. **`wrds-dl`** on your PATH — either the [Go binary](../go/) or the [Python CLI](../python/) (`uv tool install wrds-dl --from ./python`)
2. **WRDS credentials** configured via environment variables, saved credentials, or `~/.pgpass`

## Usage

In Claude Code, type:

```
/wrds-download CRSP daily stock data for 2020
```

```
/wrds-download Compustat annual fundamentals, gvkey datadate and sales, 2010-2023
```

```
/wrds-download IBES analyst EPS estimates for Apple
```

Claude will inspect the table, show you the structure and row count, do a dry run for large tables, and download the data to a local Parquet file.
