"""Tests for CLI commands — help output and flag parsing."""

from click.testing import CliRunner

from wrds_dl.cli import cli


def test_cli_help():
    runner = CliRunner()
    result = runner.invoke(cli, ["--help"])
    assert result.exit_code == 0
    assert "download" in result.output
    assert "info" in result.output


def test_download_help():
    runner = CliRunner()
    result = runner.invoke(cli, ["download", "--help"])
    assert result.exit_code == 0
    for flag in ["--schema", "--table", "--columns", "--where", "--query", "--out", "--format",
                 "--limit", "--dry-run"]:
        assert flag in result.output


def test_info_help():
    runner = CliRunner()
    result = runner.invoke(cli, ["info", "--help"])
    assert result.exit_code == 0
    for flag in ["--schema", "--table", "--json"]:
        assert flag in result.output


def test_download_no_args():
    runner = CliRunner()
    result = runner.invoke(cli, ["download"])
    assert result.exit_code != 0
    assert "Either --query or both --schema and --table" in result.output
