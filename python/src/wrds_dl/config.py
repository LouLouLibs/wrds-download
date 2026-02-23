"""Credentials file I/O — reads/writes ~/.config/wrds-dl/credentials."""

from __future__ import annotations

import os
from pathlib import Path


def credentials_path() -> Path:
    """Return the path to the credentials file, respecting $XDG_CONFIG_HOME."""
    base = os.environ.get("XDG_CONFIG_HOME") or Path.home() / ".config"
    return Path(base) / "wrds-dl" / "credentials"


def load_credentials() -> tuple[str, str, str]:
    """Read PGUSER, PGPASSWORD, PGDATABASE from the credentials file.

    Returns (user, password, database). Missing values are empty strings.
    """
    user = password = database = ""
    path = credentials_path()
    if not path.is_file():
        return user, password, database

    for line in path.read_text().splitlines():
        line = line.strip()
        if not line or line.startswith("#"):
            continue
        key, _, val = line.partition("=")
        key, val = key.strip(), val.strip()
        if key == "PGUSER":
            user = val
        elif key == "PGPASSWORD":
            password = val
        elif key == "PGDATABASE":
            database = val
    return user, password, database


def apply_credentials() -> None:
    """Load credentials from config and set env vars for any values not already set."""
    user, password, database = load_credentials()
    if not os.environ.get("PGUSER") and user:
        os.environ["PGUSER"] = user
    if not os.environ.get("PGPASSWORD") and password:
        os.environ["PGPASSWORD"] = password
    if not os.environ.get("PGDATABASE") and database:
        os.environ["PGDATABASE"] = database
