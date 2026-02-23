"""Tests for config module — credentials file parsing."""

import os
from pathlib import Path
from unittest import mock

from wrds_dl.config import apply_credentials, credentials_path, load_credentials


class TestCredentialsPath:
    def test_default(self):
        with mock.patch.dict(os.environ, {}, clear=True):
            path = credentials_path()
            assert path == Path.home() / ".config" / "wrds-dl" / "credentials"

    def test_xdg(self):
        with mock.patch.dict(os.environ, {"XDG_CONFIG_HOME": "/tmp/xdg"}):
            path = credentials_path()
            assert path == Path("/tmp/xdg/wrds-dl/credentials")


class TestLoadCredentials:
    def test_missing_file(self, tmp_path):
        with mock.patch("wrds_dl.config.credentials_path", return_value=tmp_path / "missing"):
            user, pw, db = load_credentials()
            assert user == ""
            assert pw == ""
            assert db == ""

    def test_valid_file(self, tmp_path):
        creds = tmp_path / "credentials"
        creds.write_text("PGUSER=alice\nPGPASSWORD=secret\nPGDATABASE=wrds\n")
        with mock.patch("wrds_dl.config.credentials_path", return_value=creds):
            user, pw, db = load_credentials()
            assert user == "alice"
            assert pw == "secret"
            assert db == "wrds"

    def test_comments_and_blanks(self, tmp_path):
        creds = tmp_path / "credentials"
        creds.write_text("# comment\n\nPGUSER=bob\n")
        with mock.patch("wrds_dl.config.credentials_path", return_value=creds):
            user, pw, db = load_credentials()
            assert user == "bob"
            assert pw == ""


class TestApplyCredentials:
    def test_sets_missing_env(self, tmp_path):
        creds = tmp_path / "credentials"
        creds.write_text("PGUSER=alice\nPGPASSWORD=secret\n")
        with mock.patch("wrds_dl.config.credentials_path", return_value=creds):
            env = {"PATH": os.environ.get("PATH", "")}
            with mock.patch.dict(os.environ, env, clear=True):
                apply_credentials()
                assert os.environ["PGUSER"] == "alice"
                assert os.environ["PGPASSWORD"] == "secret"

    def test_does_not_override(self, tmp_path):
        creds = tmp_path / "credentials"
        creds.write_text("PGUSER=alice\n")
        with mock.patch("wrds_dl.config.credentials_path", return_value=creds):
            with mock.patch.dict(os.environ, {"PGUSER": "existing"}):
                apply_credentials()
                assert os.environ["PGUSER"] == "existing"
